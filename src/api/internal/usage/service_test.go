package usage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"copilot-per-user/api/internal/budget"
	gh "copilot-per-user/api/internal/github"
)

type fakeResolver struct {
	mu            sync.Mutex
	login         string
	loginsByEmail map[string]string
	err           error
	emails        []string
}

func (f *fakeResolver) ResolveGitHubLogin(_ context.Context, email string) (string, error) {
	f.mu.Lock()
	f.emails = append(f.emails, email)
	f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if f.loginsByEmail != nil {
		return f.loginsByEmail[email], nil
	}
	return f.login, nil
}

type fakeBilling struct {
	mu           sync.Mutex
	requests     []gh.AICreditUsageRequest
	report       gh.AICreditUsageReport
	reportsByDay map[int]gh.AICreditUsageReport
	err          error
	wait         <-chan struct{}
}

func (f *fakeBilling) GetAICreditUsage(_ context.Context, req gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.wait != nil {
		<-f.wait
	}
	if f.err != nil {
		return gh.AICreditUsageReport{}, f.err
	}
	if req.Day > 0 {
		if f.reportsByDay != nil {
			return f.reportsByDay[req.Day], nil
		}
		return gh.AICreditUsageReport{}, nil
	}
	return f.report, nil
}

func (f *fakeBilling) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

type fakeBudgetResolver struct {
	mu     sync.Mutex
	logins []string
	result budget.UserBudget
	err    error
}

func (f *fakeBudgetResolver) ResolveUserBudget(_ context.Context, login string) (budget.UserBudget, error) {
	f.mu.Lock()
	f.logins = append(f.logins, login)
	f.mu.Unlock()
	if f.err != nil {
		return budget.UserBudget{}, f.err
	}
	return f.result, nil
}

func TestServiceReturnsNormalizedUserUsage(t *testing.T) {
	report := gh.AICreditUsageReport{
		Enterprise: "marbis",
		UsageItems: []gh.AICreditUsageItem{
			{Model: "GPT-5.5", PricePerUnit: 0.01, DiscountQuantity: 10, NetQuantity: 2, GrossAmount: 0.12, NetAmount: 0.02},
			{Model: "Claude", PricePerUnit: 0.01, DiscountQuantity: 3, NetQuantity: 4, GrossAmount: 0.07, NetAmount: 0.04},
		},
	}
	billing := &fakeBilling{report: report}
	resolver := &fakeResolver{login: "Annonator"}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	result, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("GetMonthlyUsage() error = %v", err)
	}
	if result.User.Email != "andreas.pohl@nitrado.net" {
		t.Fatalf("Email = %q", result.User.Email)
	}
	if result.User.GitHubLogin != "Annonator" {
		t.Fatalf("GitHubLogin = %q", result.User.GitHubLogin)
	}
	if result.Totals.IncludedCredits != 13 {
		t.Fatalf("IncludedCredits = %.2f", result.Totals.IncludedCredits)
	}
	if result.Totals.AdditionalCredits != 6 {
		t.Fatalf("AdditionalCredits = %.2f", result.Totals.AdditionalCredits)
	}
	if result.Totals.GrossAmount != 0.19 {
		t.Fatalf("GrossAmount = %.2f", result.Totals.GrossAmount)
	}
	if result.Totals.AdditionalUsage != 0.06 {
		t.Fatalf("AdditionalUsage = %.2f", result.Totals.AdditionalUsage)
	}
	if len(result.Models) != 2 {
		t.Fatalf("Models length = %d", len(result.Models))
	}
	if result.Models[0].IncludedCredits != 10 {
		t.Fatalf("first model IncludedCredits = %.2f", result.Models[0].IncludedCredits)
	}
	if result.Models[0].AdditionalCredits != 2 {
		t.Fatalf("first model AdditionalCredits = %.2f", result.Models[0].AdditionalCredits)
	}
	if result.Models[0].PricePerCredit != 0.01 {
		t.Fatalf("first model PricePerCredit = %.2f", result.Models[0].PricePerCredit)
	}
	if len(result.Daily) != now.Day() {
		t.Fatalf("Daily length = %d", len(result.Daily))
	}
	if len(billing.requests) != expectedBillingRequestCount(now, 2026, 6) {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if billing.requests[0].User != "Annonator" {
		t.Fatalf("billing user = %q", billing.requests[0].User)
	}
	if billing.requests[0].Day != 0 {
		t.Fatalf("monthly request Day = %d", billing.requests[0].Day)
	}
	for _, req := range billing.requests {
		if req.User != "Annonator" {
			t.Fatalf("billing request user = %q", req.User)
		}
	}
	if len(resolver.emails) != 1 || resolver.emails[0] != "andreas.pohl@nitrado.net" {
		t.Fatalf("resolved emails = %#v", resolver.emails)
	}
	if result.SourceMetadata.Enterprise != "marbis" {
		t.Fatalf("source enterprise = %q", result.SourceMetadata.Enterprise)
	}
	if result.SourceMetadata.Source != "github_enterprise_billing_ai_credit_usage" {
		t.Fatalf("source = %q", result.SourceMetadata.Source)
	}
	if result.SourceMetadata.Cached {
		t.Fatal("Cached = true, want false")
	}
}

func TestServiceIncludesResolvedBudget(t *testing.T) {
	billing := &fakeBilling{}
	monthlyLimit := 30.0
	consumed := 12.5
	remaining := 17.5
	usagePercent := 41.66666666666667
	preventFurtherUsage := true
	budgetResolver := &fakeBudgetResolver{
		result: budget.UserBudget{
			Status:              budget.StatusAvailable,
			Source:              budget.SourceUniversal,
			BudgetID:            "universal-ai",
			MonthlyLimitUSD:     &monthlyLimit,
			ConsumedUSD:         &consumed,
			RemainingUSD:        &remaining,
			UsagePercent:        &usagePercent,
			PreventFurtherUsage: &preventFurtherUsage,
		},
	}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		Budget:     budgetResolver,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
	})

	result, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("GetMonthlyUsage() error = %v", err)
	}
	if result.Budget == nil {
		t.Fatal("Budget = nil, want resolved budget")
	}
	if result.Budget.Status != budget.StatusAvailable {
		t.Fatalf("budget status = %q", result.Budget.Status)
	}
	if result.Budget.BudgetID != "universal-ai" {
		t.Fatalf("budget ID = %q", result.Budget.BudgetID)
	}
	if result.Budget.MonthlyLimitUSD == nil || *result.Budget.MonthlyLimitUSD != 30 {
		t.Fatalf("MonthlyLimitUSD = %#v", result.Budget.MonthlyLimitUSD)
	}
	if len(budgetResolver.logins) != 1 || budgetResolver.logins[0] != "Annonator" {
		t.Fatalf("budget resolver logins = %#v", budgetResolver.logins)
	}
}

func TestServiceMarksBudgetUnavailableWhenBudgetLookupFails(t *testing.T) {
	billing := &fakeBilling{}
	budgetResolver := &fakeBudgetResolver{err: errors.New("budget API unavailable")}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		Budget:     budgetResolver,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
	})

	result, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("GetMonthlyUsage() error = %v", err)
	}
	if result.Budget == nil {
		t.Fatal("Budget = nil, want unavailable budget status")
	}
	if result.Budget.Status != budget.StatusUnavailable {
		t.Fatalf("budget status = %q", result.Budget.Status)
	}
	if result.Totals != (UsageTotals{}) {
		t.Fatalf("usage totals = %#v, want usage response preserved", result.Totals)
	}
}

func TestServiceFetchesDailyUsageThroughElapsedCurrentMonthDays(t *testing.T) {
	billing := &fakeBilling{}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	result, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("GetMonthlyUsage() error = %v", err)
	}
	if len(result.Daily) != now.Day() {
		t.Fatalf("Daily length = %d, want elapsed days", len(result.Daily))
	}
	if billing.requestCount() != expectedBillingRequestCount(now, 2026, 6) {
		t.Fatalf("billing request count = %d, want monthly plus elapsed daily requests", billing.requestCount())
	}
	if billing.requests[0].Day != 0 {
		t.Fatalf("billing request day = %d, want monthly request", billing.requests[0].Day)
	}
	if got := billing.requests[len(billing.requests)-1].Day; got != now.Day() {
		t.Fatalf("last daily request day = %d, want %d", got, now.Day())
	}
}

func TestServicePopulatesDailyUsageFromDayFilteredBilling(t *testing.T) {
	monthlyReport := gh.AICreditUsageReport{
		Enterprise: "marbis",
		UsageItems: []gh.AICreditUsageItem{
			{Model: "GPT-5.5", PricePerUnit: 0.01, DiscountQuantity: 30, NetQuantity: 12, GrossAmount: 0.42, NetAmount: 0.12},
		},
	}
	billing := &fakeBilling{
		report: monthlyReport,
		reportsByDay: map[int]gh.AICreditUsageReport{
			1: {
				UsageItems: []gh.AICreditUsageItem{
					{Model: "GPT-5.5", PricePerUnit: 0.01, DiscountQuantity: 10, NetQuantity: 2, GrossAmount: 0.12, NetAmount: 0.02},
				},
			},
			2: {
				UsageItems: []gh.AICreditUsageItem{
					{Model: "Claude", PricePerUnit: 0.01, DiscountQuantity: 3, NetQuantity: 4, GrossAmount: 0.07, NetAmount: 0.04},
				},
			},
			3: {},
		},
	}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "brendan"},
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC) },
	})

	result, err := service.GetMonthlyUsage(context.Background(), "brendan@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("GetMonthlyUsage() error = %v", err)
	}
	if len(result.Daily) != 3 {
		t.Fatalf("Daily length = %d, want one row per elapsed day", len(result.Daily))
	}
	if got := result.Daily[0].Day; got != "2026-06-01" {
		t.Fatalf("first daily day = %q", got)
	}
	if got := result.Daily[0].Totals.IncludedCredits; got != 10 {
		t.Fatalf("first daily included credits = %.2f", got)
	}
	if got := result.Daily[0].Totals.AdditionalCredits; got != 2 {
		t.Fatalf("first daily additional credits = %.2f", got)
	}
	if got := result.Daily[1].Day; got != "2026-06-02" {
		t.Fatalf("second daily day = %q", got)
	}
	if got := result.Daily[1].Totals.AdditionalUsage; got != 0.04 {
		t.Fatalf("second daily additional usage = %.2f", got)
	}
	if got := result.Daily[2].Day; got != "2026-06-03" {
		t.Fatalf("third daily day = %q", got)
	}
	if result.Daily[2].Totals != (UsageTotals{}) {
		t.Fatalf("third daily totals = %#v, want empty usage", result.Daily[2].Totals)
	}
	if billing.requestCount() != 4 {
		t.Fatalf("billing request count = %d, want monthly plus elapsed daily requests", billing.requestCount())
	}
	for i, req := range billing.requests {
		if req.User != "brendan" {
			t.Fatalf("billing request %d user = %q", i, req.User)
		}
	}
	for day := 1; day <= 3; day++ {
		if got := billing.requests[day].Day; got != day {
			t.Fatalf("daily request %d day = %d", day, got)
		}
	}
}

func TestServiceCachesByEnterpriseLoginAndPeriod(t *testing.T) {
	report := gh.AICreditUsageReport{
		Enterprise: "marbis",
		UsageItems: []gh.AICreditUsageItem{
			{Model: "GPT-5.5", PricePerUnit: 0.01, DiscountQuantity: 1},
		},
	}
	billing := &fakeBilling{report: report}
	resolver := &fakeResolver{login: "Annonator"}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	first, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 5)
	if err != nil {
		t.Fatalf("first GetMonthlyUsage() error = %v", err)
	}
	second, err := service.GetMonthlyUsage(context.Background(), "ANDREAS.POHL@nitrado.net", 2026, 5)
	if err != nil {
		t.Fatalf("second GetMonthlyUsage() error = %v", err)
	}
	if len(billing.requests) != expectedBillingRequestCount(now, 2026, 5) {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if first.SourceMetadata.Cached {
		t.Fatal("first Cached = true, want false")
	}
	if !second.SourceMetadata.Cached {
		t.Fatal("second Cached = false, want true")
	}
	if second.User.GitHubLogin != "Annonator" {
		t.Fatalf("second GitHubLogin = %q", second.User.GitHubLogin)
	}
	if second.User.Email != "ANDREAS.POHL@nitrado.net" {
		t.Fatalf("second Email = %q", second.User.Email)
	}
	if len(resolver.emails) != 2 {
		t.Fatalf("resolved email count = %d", len(resolver.emails))
	}
}

func TestServiceCoalescesConcurrentRequestsForSameLoginAndPeriod(t *testing.T) {
	releaseBilling := make(chan struct{})
	billing := &fakeBilling{wait: releaseBilling}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	const requestCount = 8
	var wg sync.WaitGroup
	errs := make(chan error, requestCount)
	for range requestCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
			errs <- err
		}()
	}

	waitForBillingRequest(t, billing)
	close(releaseBilling)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("GetMonthlyUsage() error = %v", err)
		}
	}
	if billing.requestCount() != expectedBillingRequestCount(now, 2026, 6) {
		t.Fatalf("billing request count = %d, want one coalesced monthly plus daily fetch", billing.requestCount())
	}
}

func TestServiceRejectsEmptyResolvedLoginBeforeBilling(t *testing.T) {
	billing := &fakeBilling{}
	resolver := &fakeResolver{login: "   "}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err == nil {
		t.Fatal("GetMonthlyUsage() error = nil, want empty login error")
	}
	if !strings.Contains(err.Error(), "empty GitHub login") {
		t.Fatalf("error = %q, want empty GitHub login context", err)
	}
	if len(billing.requests) != 0 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
}

func TestServiceRejectsPeriodOutsideReportingWindowBeforeResolverAndBilling(t *testing.T) {
	billing := &fakeBilling{}
	resolver := &fakeResolver{login: "Annonator"}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2025, 12)
	if err == nil {
		t.Fatal("GetMonthlyUsage() error = nil, want period outside reporting window error")
	}
	if !strings.Contains(err.Error(), "outside reporting window") {
		t.Fatalf("error = %q, want reporting window context", err)
	}
	if len(resolver.emails) != 0 {
		t.Fatalf("resolved email count = %d", len(resolver.emails))
	}
	if billing.requestCount() != 0 {
		t.Fatalf("billing request count = %d", billing.requestCount())
	}
}

func TestServiceRejectsInvalidPeriodBeforeResolverAndBilling(t *testing.T) {
	tests := []struct {
		name  string
		year  int
		month int
	}{
		{name: "old year", year: 1999, month: 6},
		{name: "month zero", year: 2026, month: 0},
		{name: "month thirteen", year: 2026, month: 13},
		{name: "future same-year month", year: 2026, month: 7},
		{name: "future year", year: 2027, month: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			billing := &fakeBilling{}
			resolver := &fakeResolver{login: "Annonator"}
			service := NewService(ServiceConfig{
				Enterprise: "marbis",
				Resolver:   resolver,
				Billing:    billing,
				Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
			})

			_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", tt.year, tt.month)
			if err == nil {
				t.Fatal("GetMonthlyUsage() error = nil, want invalid period error")
			}
			if !strings.Contains(err.Error(), "invalid period") {
				t.Fatalf("error = %q, want invalid period context", err)
			}
			if len(resolver.emails) != 0 {
				t.Fatalf("resolved email count = %d", len(resolver.emails))
			}
			if len(billing.requests) != 0 {
				t.Fatalf("billing request count = %d", len(billing.requests))
			}
		})
	}
}

func waitForBillingRequest(t *testing.T, billing *fakeBilling) {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if billing.requestCount() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for billing request")
		case <-ticker.C:
		}
	}
}

func TestServicePropagatesResolverFailureWithoutBilling(t *testing.T) {
	billing := &fakeBilling{}
	resolverErr := errors.New("identity unavailable")
	resolver := &fakeResolver{err: resolverErr}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if !errors.Is(err, resolverErr) {
		t.Fatalf("error = %v, want resolver error", err)
	}
	if len(billing.requests) != 0 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
}

func TestServicePropagatesMonthlyBillingFailure(t *testing.T) {
	billingErr := errors.New("monthly billing unavailable")
	billing := &fakeBilling{err: billingErr}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if !errors.Is(err, billingErr) {
		t.Fatalf("error = %v, want monthly billing error", err)
	}
	if len(billing.requests) != 1 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if billing.requests[0].Day != 0 {
		t.Fatalf("billing request day = %d", billing.requests[0].Day)
	}
}

func TestServiceRefetchesAfterCacheExpiry(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	firstNow := now
	billing := &fakeBilling{}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("first GetMonthlyUsage() error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	second, err := service.GetMonthlyUsage(context.Background(), "ANDREAS.POHL@nitrado.net", 2026, 6)
	if err != nil {
		t.Fatalf("second GetMonthlyUsage() error = %v", err)
	}
	if len(billing.requests) != 2*expectedBillingRequestCount(firstNow, 2026, 6) {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if second.SourceMetadata.Cached {
		t.Fatal("second Cached = true, want false after expiry")
	}
	if second.User.Email != "ANDREAS.POHL@nitrado.net" {
		t.Fatalf("second Email = %q", second.User.Email)
	}
}

func TestServiceSeparatesCacheByUserAndMonth(t *testing.T) {
	billing := &fakeBilling{}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	resolver := &fakeResolver{loginsByEmail: map[string]string{
		"andreas.pohl@nitrado.net": "Annonator",
		"ada@nitrado.net":          "Ada",
	}}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return now },
	})

	cases := []struct {
		email string
		year  int
		month int
	}{
		{email: "andreas.pohl@nitrado.net", year: 2026, month: 5},
		{email: "andreas.pohl@nitrado.net", year: 2026, month: 5},
		{email: "ada@nitrado.net", year: 2026, month: 5},
		{email: "andreas.pohl@nitrado.net", year: 2026, month: 6},
	}
	for i, tc := range cases {
		result, err := service.GetMonthlyUsage(context.Background(), tc.email, tc.year, tc.month)
		if err != nil {
			t.Fatalf("GetMonthlyUsage(%d) error = %v", i, err)
		}
		if i == 1 && !result.SourceMetadata.Cached {
			t.Fatal("second same-user same-month result Cached = false, want true")
		}
	}

	expectedRequests := expectedBillingRequestCount(now, 2026, 5) +
		expectedBillingRequestCount(now, 2026, 5) +
		expectedBillingRequestCount(now, 2026, 6)
	if len(billing.requests) != expectedRequests {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	monthlyRequests := requestsForDay(billing.requests, 0)
	if len(monthlyRequests) != 3 {
		t.Fatalf("monthly billing request count = %d", len(monthlyRequests))
	}
	if got := fmt.Sprintf("%s/%d", monthlyRequests[1].User, monthlyRequests[1].Month); got != "Ada/5" {
		t.Fatalf("first different-user request = %s", got)
	}
	if got := fmt.Sprintf("%s/%d", monthlyRequests[2].User, monthlyRequests[2].Month); got != "Annonator/6" {
		t.Fatalf("first different-month request = %s", got)
	}
}

func expectedBillingRequestCount(now time.Time, year, month int) int {
	now = now.UTC()
	days := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if now.Year() == year && int(now.Month()) == month {
		days = now.Day()
	}
	return 1 + days
}

func requestsForDay(requests []gh.AICreditUsageRequest, day int) []gh.AICreditUsageRequest {
	filtered := make([]gh.AICreditUsageRequest, 0)
	for _, request := range requests {
		if request.Day == day {
			filtered = append(filtered, request)
		}
	}
	return filtered
}
