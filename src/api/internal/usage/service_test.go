package usage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	gh "copilot-per-user/api/internal/github"
)

type fakeResolver struct {
	login         string
	loginsByEmail map[string]string
	err           error
	emails        []string
}

func (f *fakeResolver) ResolveGitHubLogin(_ context.Context, email string) (string, error) {
	f.emails = append(f.emails, email)
	if f.err != nil {
		return "", f.err
	}
	if f.loginsByEmail != nil {
		return f.loginsByEmail[email], nil
	}
	return f.login, nil
}

type fakeBilling struct {
	requests []gh.AICreditUsageRequest
	report   gh.AICreditUsageReport
	err      error
	failDay  int
}

func (f *fakeBilling) GetAICreditUsage(_ context.Context, req gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return gh.AICreditUsageReport{}, f.err
	}
	if f.failDay > 0 && f.failDay == req.Day {
		return gh.AICreditUsageReport{}, errors.New("daily billing unavailable")
	}
	return f.report, nil
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
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
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
	if len(result.Daily) != 19 {
		t.Fatalf("Daily length = %d", len(result.Daily))
	}
	if result.Daily[0].Day != "2026-06-01" {
		t.Fatalf("first Daily Day = %q", result.Daily[0].Day)
	}
	if result.Daily[18].Day != "2026-06-19" {
		t.Fatalf("last Daily Day = %q", result.Daily[18].Day)
	}
	if len(billing.requests) != 20 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if billing.requests[0].User != "Annonator" {
		t.Fatalf("billing user = %q", billing.requests[0].User)
	}
	if billing.requests[0].Day != 0 {
		t.Fatalf("monthly request Day = %d", billing.requests[0].Day)
	}
	if billing.requests[1].Day != 1 || billing.requests[19].Day != 19 {
		t.Fatalf("daily request days = first %d last %d", billing.requests[1].Day, billing.requests[19].Day)
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

func TestServiceCachesByEnterpriseLoginAndPeriod(t *testing.T) {
	report := gh.AICreditUsageReport{
		Enterprise: "marbis",
		UsageItems: []gh.AICreditUsageItem{
			{Model: "GPT-5.5", PricePerUnit: 0.01, DiscountQuantity: 1},
		},
	}
	billing := &fakeBilling{report: report}
	resolver := &fakeResolver{login: "Annonator"}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	first, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 5)
	if err != nil {
		t.Fatalf("first GetMonthlyUsage() error = %v", err)
	}
	second, err := service.GetMonthlyUsage(context.Background(), "ANDREAS.POHL@nitrado.net", 2026, 5)
	if err != nil {
		t.Fatalf("second GetMonthlyUsage() error = %v", err)
	}
	if len(billing.requests) != 32 {
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

func TestServiceFetchesAllDaysForHistoricalLeapYearFebruary(t *testing.T) {
	billing := &fakeBilling{}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	result, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2024, 2)
	if err != nil {
		t.Fatalf("GetMonthlyUsage() error = %v", err)
	}
	if len(result.Daily) != 29 {
		t.Fatalf("Daily length = %d", len(result.Daily))
	}
	if result.Daily[28].Day != "2024-02-29" {
		t.Fatalf("last Daily Day = %q", result.Daily[28].Day)
	}
	if len(billing.requests) != 30 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if billing.requests[29].Day != 29 {
		t.Fatalf("last billing request day = %d", billing.requests[29].Day)
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

func TestServicePropagatesDailyBillingFailure(t *testing.T) {
	billing := &fakeBilling{failDay: 2}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   &fakeResolver{login: "Annonator"},
		Billing:    billing,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
	})

	_, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 6)
	if err == nil {
		t.Fatal("GetMonthlyUsage() error = nil, want daily billing error")
	}
	if !strings.Contains(err.Error(), "day 2") {
		t.Fatalf("error = %q, want day context", err)
	}
	if len(billing.requests) != 3 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
}

func TestServiceRefetchesAfterCacheExpiry(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
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
	if len(billing.requests) != 40 {
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
	resolver := &fakeResolver{loginsByEmail: map[string]string{
		"andreas.pohl@nitrado.net": "Annonator",
		"ada@nitrado.net":          "Ada",
	}}
	service := NewService(ServiceConfig{
		Enterprise: "marbis",
		Resolver:   resolver,
		Billing:    billing,
		CacheTTL:   time.Minute,
		Now:        func() time.Time { return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC) },
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

	if len(billing.requests) != 84 {
		t.Fatalf("billing request count = %d", len(billing.requests))
	}
	if got := fmt.Sprintf("%s/%d", billing.requests[32].User, billing.requests[32].Month); got != "Ada/5" {
		t.Fatalf("first different-user request = %s", got)
	}
	if got := fmt.Sprintf("%s/%d", billing.requests[64].User, billing.requests[64].Month); got != "Annonator/6" {
		t.Fatalf("first different-month request = %s", got)
	}
}
