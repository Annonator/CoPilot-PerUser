package usage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	gh "copilot-per-user/api/internal/github"
	"copilot-per-user/api/internal/identity"
)

const sourceGitHubEnterpriseBillingAICreditUsage = "github_enterprise_billing_ai_credit_usage"

type BillingClient interface {
	GetAICreditUsage(context.Context, gh.AICreditUsageRequest) (gh.AICreditUsageReport, error)
}

type ServiceConfig struct {
	Enterprise string
	Resolver   identity.Resolver
	Billing    BillingClient
	CacheTTL   time.Duration
	Now        func() time.Time
}

type Service struct {
	enterprise string
	resolver   identity.Resolver
	billing    BillingClient
	cacheTTL   time.Duration
	now        func() time.Time

	mu    sync.Mutex
	cache map[cacheKey]cacheEntry
}

type cacheKey struct {
	enterprise string
	login      string
	year       int
	month      int
}

type cacheEntry struct {
	expiresAt time.Time
	usage     MonthlyUsage
}

func NewService(config ServiceConfig) *Service {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		enterprise: config.Enterprise,
		resolver:   config.Resolver,
		billing:    config.Billing,
		cacheTTL:   config.CacheTTL,
		now:        now,
		cache:      make(map[cacheKey]cacheEntry),
	}
}

func (s *Service) GetMonthlyUsage(ctx context.Context, email string, year, month int) (MonthlyUsage, error) {
	if err := validatePeriod(year, month); err != nil {
		return MonthlyUsage{}, err
	}
	if s.resolver == nil {
		return MonthlyUsage{}, fmt.Errorf("usage resolver is required")
	}
	if s.billing == nil {
		return MonthlyUsage{}, fmt.Errorf("usage billing client is required")
	}

	login, err := s.resolver.ResolveGitHubLogin(ctx, email)
	if err != nil {
		return MonthlyUsage{}, fmt.Errorf("resolve GitHub login: %w", err)
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return MonthlyUsage{}, fmt.Errorf("empty GitHub login for authenticated user")
	}

	key := cacheKey{
		enterprise: s.enterprise,
		login:      login,
		year:       year,
		month:      month,
	}
	if usage, ok := s.cached(key); ok {
		usage.User.Email = email
		usage.SourceMetadata.Cached = true
		return usage, nil
	}

	monthlyReport, err := s.billing.GetAICreditUsage(ctx, gh.AICreditUsageRequest{
		Enterprise: s.enterprise,
		User:       login,
		Year:       year,
		Month:      month,
	})
	if err != nil {
		return MonthlyUsage{}, fmt.Errorf("get monthly AI credit usage: %w", err)
	}

	daily, err := s.getDailyUsage(ctx, login, year, month)
	if err != nil {
		return MonthlyUsage{}, err
	}

	models := normalizeModels(monthlyReport.UsageItems)
	usage := MonthlyUsage{
		Period: Period{
			Year:  year,
			Month: month,
		},
		User: User{
			Email:       email,
			GitHubLogin: login,
		},
		Totals: sumModels(models),
		Models: models,
		Daily:  daily,
		SourceMetadata: SourceMetadata{
			Enterprise: s.enterprise,
			Source:     sourceGitHubEnterpriseBillingAICreditUsage,
			Cached:     false,
		},
	}
	s.store(key, usage)
	return usage, nil
}

func (s *Service) getDailyUsage(ctx context.Context, login string, year, month int) ([]DailyUsage, error) {
	dayCount := s.daysToFetch(year, month)
	daily := make([]DailyUsage, 0, dayCount)
	for day := 1; day <= dayCount; day++ {
		report, err := s.billing.GetAICreditUsage(ctx, gh.AICreditUsageRequest{
			Enterprise: s.enterprise,
			User:       login,
			Year:       year,
			Month:      month,
			Day:        day,
		})
		if err != nil {
			return nil, fmt.Errorf("get daily AI credit usage for day %d: %w", day, err)
		}
		models := normalizeModels(report.UsageItems)
		daily = append(daily, DailyUsage{
			Day:    dateString(year, month, day),
			Models: models,
			Totals: sumModels(models),
		})
	}
	return daily, nil
}

func (s *Service) daysToFetch(year, month int) int {
	now := s.now().UTC()
	if year == now.Year() && month == int(now.Month()) {
		return now.Day()
	}
	return daysInMonth(year, month)
}

func validatePeriod(year, month int) error {
	if year < 2000 || month < 1 || month > 12 {
		return fmt.Errorf("invalid period: year must be >= 2000 and month must be 1..12")
	}
	return nil
}

func daysInMonth(year, month int) int {
	if month < 1 || month > 12 {
		return 0
	}
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func dateString(year, month, day int) string {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func (s *Service) cached(key cacheKey) (MonthlyUsage, bool) {
	if s.cacheTTL <= 0 {
		return MonthlyUsage{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.cache[key]
	if !ok {
		return MonthlyUsage{}, false
	}
	if !s.now().Before(entry.expiresAt) {
		delete(s.cache, key)
		return MonthlyUsage{}, false
	}
	return cloneMonthlyUsage(entry.usage), true
}

func (s *Service) store(key cacheKey, usage MonthlyUsage) {
	if s.cacheTTL <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[key] = cacheEntry{
		expiresAt: s.now().Add(s.cacheTTL),
		usage:     cloneMonthlyUsage(usage),
	}
}

func normalizeModels(items []gh.AICreditUsageItem) []ModelUsage {
	models := make([]ModelUsage, 0, len(items))
	for _, item := range items {
		models = append(models, ModelUsage{
			Model:             item.Model,
			IncludedCredits:   item.DiscountQuantity,
			AdditionalCredits: item.NetQuantity,
			GrossAmount:       item.GrossAmount,
			AdditionalUsage:   item.NetAmount,
			PricePerCredit:    item.PricePerUnit,
		})
	}
	return models
}

func sumModels(models []ModelUsage) UsageTotals {
	var totals UsageTotals
	for _, model := range models {
		totals.IncludedCredits += model.IncludedCredits
		totals.AdditionalCredits += model.AdditionalCredits
		totals.GrossAmount += model.GrossAmount
		totals.AdditionalUsage += model.AdditionalUsage
	}
	return totals
}

func cloneMonthlyUsage(usage MonthlyUsage) MonthlyUsage {
	usage.Models = append([]ModelUsage(nil), usage.Models...)
	usage.Daily = append([]DailyUsage(nil), usage.Daily...)
	for i := range usage.Daily {
		usage.Daily[i].Models = append([]ModelUsage(nil), usage.Daily[i].Models...)
	}
	return usage
}
