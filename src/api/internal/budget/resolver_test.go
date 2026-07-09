package budget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	gh "copilot-per-user/api/internal/github"
)

type fakeBudgetClient struct {
	mu                   sync.Mutex
	listRequests         []gh.BudgetListRequest
	getRequests          []gh.GetBudgetRequest
	userStateRequests    []gh.BudgetUserStatesRequest
	listResponse         gh.BudgetListResponse
	listResponsesByScope map[string]gh.BudgetListResponse
	userStateResponse    gh.BudgetUserStatesResponse
	userStateErr         error
	budgetsByID          map[string]gh.Budget
	err                  error
}

func (f *fakeBudgetClient) ListBudgets(_ context.Context, req gh.BudgetListRequest) (gh.BudgetListResponse, error) {
	f.mu.Lock()
	f.listRequests = append(f.listRequests, req)
	f.mu.Unlock()
	if f.err != nil {
		return gh.BudgetListResponse{}, f.err
	}
	if f.listResponsesByScope != nil {
		if response, ok := f.listResponsesByScope[req.Scope]; ok {
			return response, nil
		}
	}
	return f.listResponse, nil
}

func (f *fakeBudgetClient) GetBudget(_ context.Context, req gh.GetBudgetRequest) (gh.Budget, error) {
	f.mu.Lock()
	f.getRequests = append(f.getRequests, req)
	f.mu.Unlock()
	if f.err != nil {
		return gh.Budget{}, f.err
	}
	budget, ok := f.budgetsByID[req.BudgetID]
	if !ok {
		return gh.Budget{}, fmt.Errorf("missing budget %s", req.BudgetID)
	}
	return budget, nil
}

func (f *fakeBudgetClient) GetBudgetUserStates(_ context.Context, req gh.BudgetUserStatesRequest) (gh.BudgetUserStatesResponse, error) {
	f.mu.Lock()
	f.userStateRequests = append(f.userStateRequests, req)
	f.mu.Unlock()
	if f.err != nil {
		return gh.BudgetUserStatesResponse{}, f.err
	}
	if f.userStateErr != nil {
		return gh.BudgetUserStatesResponse{}, f.userStateErr
	}
	return f.userStateResponse, nil
}

func TestResolverResolvesUniversalBudgetWithoutOverride(t *testing.T) {
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{
				ID:                  "universal-ai",
				BudgetType:          "BundlePricing",
				BudgetProductSKU:    "ai_credits",
				BudgetProductSKUs:   []string{"ai_credits"},
				BudgetScope:         "multi_user_customer",
				BudgetAmount:        30,
				PreventFurtherUsage: true,
			},
		}},
		userStateResponse: gh.BudgetUserStatesResponse{UserStates: []gh.BudgetUserState{
			{User: "Annonator", ConsumedAmount: 12.5, TargetAmount: 30},
		}},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		CacheTTL:   time.Minute,
		Now:        fixedNow,
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.Status != StatusAvailable {
		t.Fatalf("Status = %q", result.Status)
	}
	if result.Source != SourceUniversal {
		t.Fatalf("Source = %q", result.Source)
	}
	if result.BudgetID != "universal-ai" {
		t.Fatalf("BudgetID = %q", result.BudgetID)
	}
	if result.ParentBudgetID != "" {
		t.Fatalf("ParentBudgetID = %q", result.ParentBudgetID)
	}
	assertFloatPointer(t, "MonthlyLimitUSD", result.MonthlyLimitUSD, 30)
	assertFloatPointer(t, "ConsumedUSD", result.ConsumedUSD, 12.5)
	assertFloatPointer(t, "RemainingUSD", result.RemainingUSD, 17.5)
	assertFloatPointer(t, "UsagePercent", result.UsagePercent, 41.66666666666667)
	assertBoolPointer(t, "PreventFurtherUsage", result.PreventFurtherUsage, true)

	if len(client.listRequests) != 1 {
		t.Fatalf("ListBudgets request count = %d", len(client.listRequests))
	}
	if client.listRequests[0].Scope != "multi_user_customer" {
		t.Fatalf("budget scope = %q", client.listRequests[0].Scope)
	}
	if client.listRequests[0].User != "Annonator" {
		t.Fatalf("budget list user = %q", client.listRequests[0].User)
	}
	if len(client.userStateRequests) != 1 {
		t.Fatalf("GetBudgetUserStates request count = %d", len(client.userStateRequests))
	}
	if client.userStateRequests[0].User != "Annonator" {
		t.Fatalf("user state request user = %q", client.userStateRequests[0].User)
	}
}

func TestResolverResolvesOverrideBudget(t *testing.T) {
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{
				ID:                  "universal-ai",
				BudgetType:          "BundlePricing",
				BudgetProductSKU:    "ai_credits",
				BudgetScope:         "multi_user_customer",
				BudgetAmount:        30,
				PreventFurtherUsage: true,
			},
		}},
		userStateResponse: gh.BudgetUserStatesResponse{UserStates: []gh.BudgetUserState{
			{User: "Annonator", ConsumedAmount: 12.5, TargetAmount: 30, OverrideBudgetID: "override-ai"},
		}},
		budgetsByID: map[string]gh.Budget{
			"override-ai": {
				ID:                  "override-ai",
				BudgetType:          "BundlePricing",
				BudgetProductSKU:    "ai_credits",
				BudgetScope:         "user",
				BudgetAmount:        50,
				PreventFurtherUsage: true,
			},
		},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		Now:        fixedNow,
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.Source != SourceOverride {
		t.Fatalf("Source = %q", result.Source)
	}
	if result.BudgetID != "override-ai" {
		t.Fatalf("BudgetID = %q", result.BudgetID)
	}
	if result.ParentBudgetID != "universal-ai" {
		t.Fatalf("ParentBudgetID = %q", result.ParentBudgetID)
	}
	assertFloatPointer(t, "MonthlyLimitUSD", result.MonthlyLimitUSD, 50)
	assertFloatPointer(t, "ConsumedUSD", result.ConsumedUSD, 12.5)
	assertFloatPointer(t, "RemainingUSD", result.RemainingUSD, 37.5)
	assertFloatPointer(t, "UsagePercent", result.UsagePercent, 25)
	if len(client.getRequests) != 1 {
		t.Fatalf("GetBudget request count = %d", len(client.getRequests))
	}
	if client.getRequests[0].BudgetID != "override-ai" {
		t.Fatalf("GetBudget BudgetID = %q", client.getRequests[0].BudgetID)
	}
}

func TestResolverIgnoresNonUniversalAndNonAICreditBudgets(t *testing.T) {
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{ID: "enterprise-ai", BudgetScope: "enterprise", BudgetProductSKU: "ai_credits", BudgetAmount: 10},
			{ID: "org-ai", BudgetScope: "organization", BudgetProductSKU: "ai_credits", BudgetAmount: 20},
			{ID: "repo-ai", BudgetScope: "repository", BudgetProductSKU: "ai_credits", BudgetAmount: 30},
			{ID: "cost-center-ai", BudgetScope: "cost_center", BudgetProductSKU: "ai_credits", BudgetAmount: 40},
			{ID: "packages", BudgetScope: "multi_user_customer", BudgetProductSKUs: []string{"packages"}, BudgetAmount: 50},
			{ID: "actions", BudgetScope: "multi_user_customer", BudgetProductSKU: "actions", BudgetAmount: 60},
			{ID: "universal-ai", BudgetType: "BundlePricing", BudgetScope: "multi_user_customer", BudgetProductSKUs: []string{"ai_credits"}, BudgetAmount: 70},
		}},
		userStateResponse: gh.BudgetUserStatesResponse{UserStates: []gh.BudgetUserState{
			{User: "Annonator", ConsumedAmount: 7, TargetAmount: 70},
		}},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		Now:        fixedNow,
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.BudgetID != "universal-ai" {
		t.Fatalf("BudgetID = %q", result.BudgetID)
	}
	assertFloatPointer(t, "MonthlyLimitUSD", result.MonthlyLimitUSD, 70)
}

func TestResolverChoosesDeterministicAICreditBudget(t *testing.T) {
	var logs []string
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{ID: "z-premium", BudgetType: "BundlePricing", BudgetScope: "multi_user_customer", BudgetProductSKU: "premium_requests", BudgetAmount: 10},
			{ID: "b-ai-sku", BudgetType: "SkuPricing", BudgetScope: "multi_user_customer", BudgetProductSKU: "ai_credits", BudgetAmount: 20},
			{ID: "a-ai-bundle", BudgetType: "BundlePricing", BudgetScope: "multi_user_customer", BudgetProductSKU: "ai_credits", BudgetAmount: 30},
		}},
		userStateResponse: gh.BudgetUserStatesResponse{UserStates: []gh.BudgetUserState{
			{User: "Annonator", ConsumedAmount: 3, TargetAmount: 30},
		}},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		Now:        fixedNow,
		Logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.BudgetID != "a-ai-bundle" {
		t.Fatalf("BudgetID = %q", result.BudgetID)
	}
	if len(logs) != 1 {
		t.Fatalf("log count = %d, want duplicate budget warning", len(logs))
	}
	if !strings.Contains(logs[0], "multiple universal AI credit budgets") {
		t.Fatalf("log = %q, want duplicate budget warning", logs[0])
	}
}

func TestResolverReturnsNotConfiguredWhenNoUniversalBudgetExists(t *testing.T) {
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{ID: "actions", BudgetScope: "multi_user_customer", BudgetProductSKU: "actions", BudgetAmount: 20},
		}},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		Now:        fixedNow,
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.Status != StatusNotConfigured {
		t.Fatalf("Status = %q", result.Status)
	}
	if result.MonthlyLimitUSD != nil {
		t.Fatalf("MonthlyLimitUSD = %.2f, want nil", *result.MonthlyLimitUSD)
	}
	if len(client.userStateRequests) != 0 {
		t.Fatalf("user state request count = %d", len(client.userStateRequests))
	}
}

func TestResolverFallsBackToUnscopedBudgetListWhenScopedResponseHasNoUniversalBudget(t *testing.T) {
	client := &fakeBudgetClient{
		listResponsesByScope: map[string]gh.BudgetListResponse{
			"multi_user_customer": {},
			"": {Budgets: []gh.Budget{
				{
					ID:                  "universal-ai",
					BudgetType:          "BundlePricing",
					BudgetProductSKU:    "ai_credits",
					BudgetScope:         "multi_user_customer",
					BudgetAmount:        30,
					PreventFurtherUsage: true,
				},
			}},
		},
		userStateResponse: gh.BudgetUserStatesResponse{UserStates: []gh.BudgetUserState{
			{User: "Annonator", ConsumedAmount: 12.5, TargetAmount: 30},
		}},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		Now:        fixedNow,
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.Status != StatusAvailable {
		t.Fatalf("Status = %q", result.Status)
	}
	if result.BudgetID != "universal-ai" {
		t.Fatalf("BudgetID = %q", result.BudgetID)
	}
	assertFloatPointer(t, "MonthlyLimitUSD", result.MonthlyLimitUSD, 30)

	if len(client.listRequests) != 2 {
		t.Fatalf("ListBudgets request count = %d, want scoped request plus unscoped fallback", len(client.listRequests))
	}
	if client.listRequests[0].Scope != "multi_user_customer" {
		t.Fatalf("first budget scope = %q", client.listRequests[0].Scope)
	}
	if client.listRequests[0].User != "Annonator" {
		t.Fatalf("first budget user = %q", client.listRequests[0].User)
	}
	if client.listRequests[1].Scope != "" {
		t.Fatalf("fallback budget scope = %q, want unscoped", client.listRequests[1].Scope)
	}
	if client.listRequests[1].User != "Annonator" {
		t.Fatalf("fallback budget user = %q", client.listRequests[1].User)
	}
	if len(client.userStateRequests) != 1 {
		t.Fatalf("GetBudgetUserStates request count = %d", len(client.userStateRequests))
	}
}

func TestResolverUsesBudgetListUserAmountsWhenUserStatesEndpointIsUnavailable(t *testing.T) {
	consumed := 0.25287777
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{
				ID:                  "universal-ai",
				BudgetType:          "BundlePricing",
				BudgetProductSKU:    "ai_credits",
				BudgetScope:         "multi_user_customer",
				BudgetAmount:        150,
				ConsumedAmount:      &consumed,
				PreventFurtherUsage: true,
			},
		}},
		userStateErr: errors.New("GitHub budget user states status 404"),
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		Now:        fixedNow,
	})

	result, err := resolver.ResolveUserBudget(context.Background(), "annonator")
	if err != nil {
		t.Fatalf("ResolveUserBudget() error = %v", err)
	}
	if result.Status != StatusAvailable {
		t.Fatalf("Status = %q", result.Status)
	}
	if result.Source != SourceUniversal {
		t.Fatalf("Source = %q", result.Source)
	}
	if result.BudgetID != "universal-ai" {
		t.Fatalf("BudgetID = %q", result.BudgetID)
	}
	assertFloatPointer(t, "MonthlyLimitUSD", result.MonthlyLimitUSD, 150)
	assertFloatPointer(t, "ConsumedUSD", result.ConsumedUSD, 0.25287777)
	assertFloatPointer(t, "RemainingUSD", result.RemainingUSD, 149.74712223)
	assertFloatPointer(t, "UsagePercent", result.UsagePercent, 0.16858518)
	assertBoolPointer(t, "PreventFurtherUsage", result.PreventFurtherUsage, true)
}

func TestResolverReturnsErrorsForBudgetAPIAndInvalidConfig(t *testing.T) {
	clientErr := errors.New("github budgets unavailable")
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     &fakeBudgetClient{err: clientErr},
		Now:        fixedNow,
	})

	_, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if !errors.Is(err, clientErr) {
		t.Fatalf("error = %v, want client error", err)
	}

	resolver = NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Now:        fixedNow,
	})
	_, err = resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err == nil || !strings.Contains(err.Error(), "budget client is required") {
		t.Fatalf("error = %v, want missing client error", err)
	}
}

func TestResolverCachesBudgetByEnterpriseAndLogin(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	client := &fakeBudgetClient{
		listResponse: gh.BudgetListResponse{Budgets: []gh.Budget{
			{ID: "universal-ai", BudgetType: "BundlePricing", BudgetScope: "multi_user_customer", BudgetProductSKU: "ai_credits", BudgetAmount: 30},
		}},
		userStateResponse: gh.BudgetUserStatesResponse{UserStates: []gh.BudgetUserState{
			{User: "Annonator", ConsumedAmount: 12.5, TargetAmount: 30},
		}},
	}
	resolver := NewResolver(ResolverConfig{
		Enterprise: "marbis",
		Client:     client,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	first, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("first ResolveUserBudget() error = %v", err)
	}
	second, err := resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("second ResolveUserBudget() error = %v", err)
	}
	if first.BudgetID != second.BudgetID {
		t.Fatalf("cached BudgetID = %q, want %q", second.BudgetID, first.BudgetID)
	}
	if len(client.listRequests) != 1 {
		t.Fatalf("ListBudgets request count = %d, want cached result", len(client.listRequests))
	}

	_, err = resolver.ResolveUserBudget(context.Background(), "Ada")
	if err != nil {
		t.Fatalf("different login ResolveUserBudget() error = %v", err)
	}
	if len(client.listRequests) != 2 {
		t.Fatalf("ListBudgets request count = %d, want separate login cache key", len(client.listRequests))
	}

	now = now.Add(2 * time.Minute)
	_, err = resolver.ResolveUserBudget(context.Background(), "Annonator")
	if err != nil {
		t.Fatalf("expired ResolveUserBudget() error = %v", err)
	}
	if len(client.listRequests) != 3 {
		t.Fatalf("ListBudgets request count = %d, want refetch after expiry", len(client.listRequests))
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
}

func assertFloatPointer(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %.4f", name, want)
	}
	const epsilon = 0.0000001
	if diff := *got - want; diff > epsilon || diff < -epsilon {
		t.Fatalf("%s = %.8f, want %.8f", name, *got, want)
	}
}

func assertBoolPointer(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %t", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %t, want %t", name, *got, want)
	}
}
