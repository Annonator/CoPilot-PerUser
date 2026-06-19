package usage

import (
	"context"
	"testing"
	"time"

	gh "copilot-per-user/api/internal/github"
)

type fakeResolver struct {
	login  string
	emails []string
}

func (f *fakeResolver) ResolveGitHubLogin(_ context.Context, email string) (string, error) {
	f.emails = append(f.emails, email)
	return f.login, nil
}

type fakeBilling struct {
	requests []gh.AICreditUsageRequest
	report   gh.AICreditUsageReport
}

func (f *fakeBilling) GetAICreditUsage(_ context.Context, req gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
	f.requests = append(f.requests, req)
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
	second, err := service.GetMonthlyUsage(context.Background(), "andreas.pohl@nitrado.net", 2026, 5)
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
	if len(resolver.emails) != 2 {
		t.Fatalf("resolved email count = %d", len(resolver.emails))
	}
}
