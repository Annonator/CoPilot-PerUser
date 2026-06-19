package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"copilot-per-user/api/internal/auth"
	"copilot-per-user/api/internal/usage"
)

type fakeUsageService struct {
	seenEmail string
	seenYear  int
	seenMonth int
	err       error
}

func (f *fakeUsageService) GetMonthlyUsage(_ context.Context, email string, year int, month int) (usage.MonthlyUsage, error) {
	f.seenEmail = email
	f.seenYear = year
	f.seenMonth = month
	if f.err != nil {
		return usage.MonthlyUsage{}, f.err
	}
	return usage.MonthlyUsage{
		Period: usage.Period{Year: year, Month: month},
		User:   usage.User{Email: email, GitHubLogin: "Annonator"},
		Totals: usage.UsageTotals{IncludedCredits: 1, AdditionalCredits: 2, GrossAmount: 0.03, AdditionalUsage: 0.02},
	}, nil
}

func TestHealthzReturnsOKWithoutAuth(t *testing.T) {
	server := NewServer(testServerConfig(&fakeUsageService{}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %#v", body)
	}
}

func TestMeReturnsAuthenticatedClaims(t *testing.T) {
	server := NewServer(testServerConfig(&fakeUsageService{}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t, "andreas.pohl@nitrado.net", "Andreas Pohl"))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Email != "andreas.pohl@nitrado.net" {
		t.Fatalf("email = %q", body.Email)
	}
	if body.Name != "Andreas Pohl" {
		t.Fatalf("name = %q", body.Name)
	}
}

func TestMeRejectsMissingOrInvalidAuth(t *testing.T) {
	server := NewServer(testServerConfig(&fakeUsageService{}))

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing auth"},
		{name: "invalid auth", authorization: "Bearer not-a-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			server.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
			}
			assertJSONError(t, resp, "unauthorized")
		})
	}
}

func TestMeRejectsDisallowedDomain(t *testing.T) {
	server := NewServer(testServerConfig(&fakeUsageService{}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t, "andreas@example.com", "Andreas Pohl"))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusForbidden)
	}
	assertJSONError(t, resp, "forbidden")
}

func TestUsageReturnsMonthlyUsageForAuthenticatedEmail(t *testing.T) {
	usageService := &fakeUsageService{}
	server := NewServer(testServerConfig(usageService))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage?year=2026&month=6", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t, "andreas.pohl@nitrado.net", "Andreas Pohl"))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if usageService.seenEmail != "andreas.pohl@nitrado.net" {
		t.Fatalf("usage email = %q", usageService.seenEmail)
	}
	if usageService.seenYear != 2026 || usageService.seenMonth != 6 {
		t.Fatalf("usage period = %d-%d", usageService.seenYear, usageService.seenMonth)
	}
	var body usage.MonthlyUsage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.User.Email != "andreas.pohl@nitrado.net" {
		t.Fatalf("response email = %q", body.User.Email)
	}
	if body.User.GitHubLogin != "Annonator" {
		t.Fatalf("response GitHubLogin = %q", body.User.GitHubLogin)
	}
	if body.Totals.IncludedCredits != 1 {
		t.Fatalf("included credits = %.2f", body.Totals.IncludedCredits)
	}
}

func TestUsageIgnoresIdentityOverrideQueryParams(t *testing.T) {
	usageService := &fakeUsageService{}
	server := NewServer(testServerConfig(usageService))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage?year=2026&month=6&user=someoneelse&email=attacker@nitrado.net", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t, "andreas.pohl@nitrado.net", "Andreas Pohl"))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if usageService.seenEmail != "andreas.pohl@nitrado.net" {
		t.Fatalf("usage email = %q, want token email", usageService.seenEmail)
	}
}

func TestUsageRejectsBadPeriod(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "bad year", path: "/v1/usage?year=bad&month=6"},
		{name: "bad month", path: "/v1/usage?year=2026&month=13"},
		{name: "missing year", path: "/v1/usage?month=6"},
		{name: "missing month", path: "/v1/usage?year=2026"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageService := &fakeUsageService{}
			server := NewServer(testServerConfig(usageService))

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+testToken(t, "andreas.pohl@nitrado.net", "Andreas Pohl"))
			server.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
			}
			if usageService.seenEmail != "" {
				t.Fatalf("usage email = %q, want no service call", usageService.seenEmail)
			}
			assertJSONError(t, resp, "bad_request")
		})
	}
}

func TestUsageReturnsUnauthorizedAndForbiddenBeforeLookup(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		wantStatus    int
		wantError     string
	}{
		{name: "missing auth", wantStatus: http.StatusUnauthorized, wantError: "unauthorized"},
		{name: "forbidden domain", authorization: "token:andreas@example.com", wantStatus: http.StatusForbidden, wantError: "forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageService := &fakeUsageService{}
			server := NewServer(testServerConfig(usageService))

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/usage?year=2026&month=6", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", "Bearer "+testToken(t, strings.TrimPrefix(tt.authorization, "token:"), "Andreas Pohl"))
			}
			server.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.Code, tt.wantStatus)
			}
			if usageService.seenEmail != "" {
				t.Fatalf("usage email = %q, want no service call", usageService.seenEmail)
			}
			assertJSONError(t, resp, tt.wantError)
		})
	}
}

func TestUsageReturnsBadGatewayForLookupFailure(t *testing.T) {
	usageService := &fakeUsageService{err: errors.New("github admin token leaked")}
	server := NewServer(testServerConfig(usageService))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage?year=2026&month=6", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t, "andreas.pohl@nitrado.net", "Andreas Pohl"))
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadGateway)
	}
	assertJSONError(t, resp, "bad_gateway")
	if strings.Contains(resp.Body.String(), "github admin token leaked") {
		t.Fatalf("response exposed internal error: %s", resp.Body.String())
	}
}

func testServerConfig(usageService UsageService) ServerConfig {
	return ServerConfig{
		Auth: auth.Manager{
			Secret: []byte("test-secret"),
			Now:    testNow,
		},
		CompanyEmailDomains: []string{"nitrado.net"},
		Usage:               usageService,
	}
}

func testToken(t *testing.T, email string, name string) string {
	t.Helper()

	token, err := (auth.Manager{Secret: []byte("test-secret"), Now: testNow}).Sign(auth.Claims{
		Email: email,
		Name:  name,
	}, time.Hour)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return token
}

func testNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}

func assertJSONError(t *testing.T, resp *httptest.ResponseRecorder, want string) {
	t.Helper()

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error != want {
		t.Fatalf("error = %q, want %q", body.Error, want)
	}
}
