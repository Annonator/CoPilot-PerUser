package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type usageBillingClient interface {
	GetAICreditUsage(context.Context, AICreditUsageRequest) (AICreditUsageReport, error)
}

var _ usageBillingClient = (*BillingClient)(nil)

func TestBillingClientRequestsFilteredEnterpriseUsage(t *testing.T) {
	var requestedPath string
	var requestedQuery string
	var authorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "../testfixtures/ai-credit-usage.json")
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	report, err := client.GetAICreditUsage(context.Background(), AICreditUsageRequest{
		Enterprise: "marbis",
		User:       "Annonator",
		Year:       2026,
		Month:      6,
	})
	if err != nil {
		t.Fatalf("GetAICreditUsage() error = %v", err)
	}
	if requestedPath != "/enterprises/marbis/settings/billing/ai_credit/usage" {
		t.Fatalf("path = %q", requestedPath)
	}
	if requestedQuery != "month=6&user=Annonator&year=2026" {
		t.Fatalf("query = %q", requestedQuery)
	}
	if authorization != "Bearer secret-token" {
		t.Fatalf("authorization = %q", authorization)
	}
	if len(report.UsageItems) != 2 {
		t.Fatalf("UsageItems length = %d", len(report.UsageItems))
	}
	if report.UsageItems[0].Model != "GPT-5.5" {
		t.Fatalf("first model = %q", report.UsageItems[0].Model)
	}
}

func TestBillingClientReturnsErrorForNon2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	_, err := client.GetAICreditUsage(context.Background(), AICreditUsageRequest{
		Enterprise: "marbis",
		User:       "Annonator",
		Year:       2026,
		Month:      6,
	})
	if err == nil {
		t.Fatal("GetAICreditUsage() error = nil, want non-2xx status error")
	}
	if !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("error = %q, want status 429", err)
	}
}

func TestBillingClientReturnsErrorForMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not-json"))
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	_, err := client.GetAICreditUsage(context.Background(), AICreditUsageRequest{
		Enterprise: "marbis",
		User:       "Annonator",
		Year:       2026,
		Month:      6,
	})
	if err == nil {
		t.Fatal("GetAICreditUsage() error = nil, want decode error")
	}
	if !strings.Contains(err.Error(), "decode GitHub billing usage") {
		t.Fatalf("error = %q, want decode context", err)
	}
}

func TestBillingClientOnlyIncludesPositiveNumericFilters(t *testing.T) {
	var requestedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usageItems":[]}`))
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	_, err := client.GetAICreditUsage(context.Background(), AICreditUsageRequest{
		Enterprise: "marbis",
		User:       "Annonator",
		Year:       -2026,
		Month:      0,
		Day:        -1,
	})
	if err != nil {
		t.Fatalf("GetAICreditUsage() error = %v", err)
	}
	if requestedQuery != "user=Annonator" {
		t.Fatalf("query = %q", requestedQuery)
	}
}
