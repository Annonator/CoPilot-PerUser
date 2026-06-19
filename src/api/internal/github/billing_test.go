package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
