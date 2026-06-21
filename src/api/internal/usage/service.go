package usage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gh "copilot-per-user/api/internal/github"
	"copilot-per-user/api/internal/identity"
)

const sourceGitHubEnterpriseBillingAICreditUsage = "github_enterprise_billing_ai_credit_usage"

const DefaultReportingWindowMonths = 6

var (
	ErrInvalidPeriod    = errors.New("invalid period")
	ErrPeriodOutOfRange = errors.New("period outside reporting window")
)

type BillingClient interface {
	GetAICreditUsage(context.Context, gh.AICreditUsageRequest) (gh.AICreditUsageReport, error)
}

type ServiceConfig struct {
	Enterprise string
	Resolver   identity.Resolver
	Billing    BillingClient
	CacheTTL   time.Duration
	Now        func() time.Time

	ReportingWindowMonths int
}

type Service struct {
	enterprise string
	resolver   identity.Resolver
	billing    BillingClient
	cacheTTL   time.Duration
	now        func() time.Time

	reportingWindowMonths int

	mu       sync.Mutex
	cache    map[cacheKey]cacheEntry
	inflight map[cacheKey]*inflightCall
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

type inflightCall struct {
	done  chan struct{}
	usage MonthlyUsage
	err   error
}

func NewService(config ServiceConfig) *Service {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		enterprise:            config.Enterprise,
		resolver:              config.Resolver,
		billing:               config.Billing,
		cacheTTL:              config.CacheTTL,
		now:                   now,
		reportingWindowMonths: normalizeReportingWindowMonths(config.ReportingWindowMonths),
		cache:                 make(map[cacheKey]cacheEntry),
		inflight:              make(map[cacheKey]*inflightCall),
	}
}

func (s *Service) GetMonthlyUsage(ctx context.Context, email string, year, month int) (MonthlyUsage, error) {
	if err := ValidateReportingPeriod(year, month, s.now(), s.reportingWindowMonths); err != nil {
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

	call, owner := s.getInflight(key)
	if !owner {
		<-call.done
		if call.err != nil {
			return MonthlyUsage{}, call.err
		}
		usage := cloneMonthlyUsage(call.usage)
		usage.User.Email = email
		return usage, nil
	}

	usage, err := s.fetchMonthlyUsage(ctx, email, login, year, month)
	if err == nil {
		s.store(key, usage)
	}
	s.finishInflight(key, call, usage, err)
	if err != nil {
		return MonthlyUsage{}, err
	}
	return usage, nil
}

func (s *Service) fetchMonthlyUsage(ctx context.Context, email, login string, year, month int) (MonthlyUsage, error) {
	monthlyReport, err := s.billing.GetAICreditUsage(ctx, gh.AICreditUsageRequest{
		Enterprise: s.enterprise,
		User:       login,
		Year:       year,
		Month:      month,
	})
	if err != nil {
		return MonthlyUsage{}, fmt.Errorf("get monthly AI credit usage: %w", err)
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
		Daily:  []DailyUsage{},
		SourceMetadata: SourceMetadata{
			Enterprise: s.enterprise,
			Source:     sourceGitHubEnterpriseBillingAICreditUsage,
			Cached:     false,
		},
	}
	return usage, nil
}

func ValidateReportingPeriod(year, month int, now time.Time, reportingWindowMonths int) error {
	if year < 2000 || month < 1 || month > 12 {
		return fmt.Errorf("%w: year must be >= 2000 and month must be 1..12", ErrInvalidPeriod)
	}
	now = now.UTC()
	if year > now.Year() || (year == now.Year() && month > int(now.Month())) {
		return fmt.Errorf("%w: period must not be after the current month", ErrInvalidPeriod)
	}
	reportingWindowMonths = normalizeReportingWindowMonths(reportingWindowMonths)
	requestedMonth := serialMonth(year, month)
	currentMonth := serialMonth(now.Year(), int(now.Month()))
	oldestAllowedMonth := currentMonth - reportingWindowMonths + 1
	if requestedMonth < oldestAllowedMonth {
		return fmt.Errorf("%w: period must be within the most recent %d months", ErrPeriodOutOfRange, reportingWindowMonths)
	}
	return nil
}

func normalizeReportingWindowMonths(months int) int {
	if months <= 0 {
		return DefaultReportingWindowMonths
	}
	return months
}

func serialMonth(year, month int) int {
	return year*12 + month - 1
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

func (s *Service) getInflight(key cacheKey) (*inflightCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if call, ok := s.inflight[key]; ok {
		return call, false
	}
	call := &inflightCall{done: make(chan struct{})}
	s.inflight[key] = call
	return call, true
}

func (s *Service) finishInflight(key cacheKey, call *inflightCall, usage MonthlyUsage, err error) {
	call.usage = cloneMonthlyUsage(usage)
	call.err = err

	close(call.done)

	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()
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
	usage.Models = cloneModels(usage.Models)
	usage.Daily = cloneDailyUsage(usage.Daily)
	for i := range usage.Daily {
		usage.Daily[i].Models = cloneModels(usage.Daily[i].Models)
	}
	return usage
}

func cloneModels(models []ModelUsage) []ModelUsage {
	if models == nil {
		return nil
	}
	out := make([]ModelUsage, len(models))
	copy(out, models)
	return out
}

func cloneDailyUsage(daily []DailyUsage) []DailyUsage {
	if daily == nil {
		return nil
	}
	out := make([]DailyUsage, len(daily))
	copy(out, daily)
	return out
}
