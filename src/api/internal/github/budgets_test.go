package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type usageBudgetClient interface {
	ListBudgets(context.Context, BudgetListRequest) (BudgetListResponse, error)
	GetBudget(context.Context, GetBudgetRequest) (Budget, error)
	GetBudgetUserStates(context.Context, BudgetUserStatesRequest) (BudgetUserStatesResponse, error)
}

var _ usageBudgetClient = (*BillingClient)(nil)

func TestBillingClientListsEnterpriseBudgets(t *testing.T) {
	var requestedPath string
	var requestedQuery string
	var authorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"budgets": [
				{
					"id": "universal-ai",
					"budget_type": "BundlePricing",
					"budget_product_sku": "ai_credits",
					"budget_product_skus": ["ai_credits"],
					"budget_scope": "multi_user_customer",
					"budget_amount": 30,
					"prevent_further_usage": true
				}
			],
			"has_next_page": false,
			"total_count": 1
		}`))
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	response, err := client.ListBudgets(context.Background(), BudgetListRequest{
		Enterprise: "marbis",
		Scope:      "multi_user_customer",
		User:       "Annonator",
		Page:       2,
		PerPage:    100,
	})
	if err != nil {
		t.Fatalf("ListBudgets() error = %v", err)
	}
	if requestedPath != "/enterprises/marbis/settings/billing/budgets" {
		t.Fatalf("path = %q", requestedPath)
	}
	if requestedQuery != "page=2&per_page=100&scope=multi_user_customer&user=Annonator" {
		t.Fatalf("query = %q", requestedQuery)
	}
	if authorization != "Bearer secret-token" {
		t.Fatalf("authorization = %q", authorization)
	}
	if len(response.Budgets) != 1 {
		t.Fatalf("Budgets length = %d", len(response.Budgets))
	}
	if response.Budgets[0].ID != "universal-ai" {
		t.Fatalf("budget ID = %q", response.Budgets[0].ID)
	}
	if response.Budgets[0].BudgetAmount != 30 {
		t.Fatalf("budget amount = %.2f", response.Budgets[0].BudgetAmount)
	}
	if !response.Budgets[0].PreventFurtherUsage {
		t.Fatal("PreventFurtherUsage = false, want true")
	}
}

func TestBillingClientGetsBudgetByID(t *testing.T) {
	var requestedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "override-ai",
			"budget_type": "BundlePricing",
			"budget_product_sku": "ai_credits",
			"budget_scope": "user",
			"budget_amount": 50,
			"prevent_further_usage": true
		}`))
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	budget, err := client.GetBudget(context.Background(), GetBudgetRequest{
		Enterprise: "marbis",
		BudgetID:   "override-ai",
	})
	if err != nil {
		t.Fatalf("GetBudget() error = %v", err)
	}
	if requestedPath != "/enterprises/marbis/settings/billing/budgets/override-ai" {
		t.Fatalf("path = %q", requestedPath)
	}
	if budget.ID != "override-ai" {
		t.Fatalf("ID = %q", budget.ID)
	}
	if budget.BudgetScope != "user" {
		t.Fatalf("BudgetScope = %q", budget.BudgetScope)
	}
}

func TestBillingClientGetsBudgetUserStates(t *testing.T) {
	var requestedPath string
	var requestedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"user_states": [
				{
					"user": "Annonator",
					"consumed_amount": 12.5,
					"target_amount": 30,
					"override_budget_id": "override-ai"
				}
			],
			"has_next_page": false,
			"total_count": 1
		}`))
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	response, err := client.GetBudgetUserStates(context.Background(), BudgetUserStatesRequest{
		Enterprise: "marbis",
		BudgetID:   "universal-ai",
		User:       "Annonator",
		PerPage:    100,
	})
	if err != nil {
		t.Fatalf("GetBudgetUserStates() error = %v", err)
	}
	if requestedPath != "/enterprises/marbis/settings/billing/budgets/universal-ai/user-states" {
		t.Fatalf("path = %q", requestedPath)
	}
	if requestedQuery != "per_page=100&user=Annonator" {
		t.Fatalf("query = %q", requestedQuery)
	}
	if len(response.UserStates) != 1 {
		t.Fatalf("UserStates length = %d", len(response.UserStates))
	}
	state := response.UserStates[0]
	if state.User != "Annonator" {
		t.Fatalf("User = %q", state.User)
	}
	if state.ConsumedAmount != 12.5 {
		t.Fatalf("ConsumedAmount = %.2f", state.ConsumedAmount)
	}
	if state.TargetAmount != 30 {
		t.Fatalf("TargetAmount = %.2f", state.TargetAmount)
	}
	if state.OverrideBudgetID != "override-ai" {
		t.Fatalf("OverrideBudgetID = %q", state.OverrideBudgetID)
	}
}

func TestBillingBudgetMethodsReturnStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewBillingClient(server.URL, "secret-token", http.DefaultClient)
	_, err := client.ListBudgets(context.Background(), BudgetListRequest{Enterprise: "marbis"})
	if err == nil {
		t.Fatal("ListBudgets() error = nil, want status error")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %q, want status 403", err)
	}

	_, err = client.GetBudget(context.Background(), GetBudgetRequest{Enterprise: "marbis", BudgetID: "budget-id"})
	if err == nil {
		t.Fatal("GetBudget() error = nil, want status error")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %q, want status 403", err)
	}

	_, err = client.GetBudgetUserStates(context.Background(), BudgetUserStatesRequest{Enterprise: "marbis", BudgetID: "budget-id"})
	if err == nil {
		t.Fatal("GetBudgetUserStates() error = nil, want status error")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %q, want status 403", err)
	}
}
