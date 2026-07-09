package budget

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	gh "copilot-per-user/api/internal/github"
)

const (
	StatusAvailable     = "available"
	StatusNotConfigured = "not_configured"
	StatusUnavailable   = "unavailable"

	SourceUniversal = "universal"
	SourceOverride  = "override"
)

const (
	budgetScopeMultiUserCustomer = "multi_user_customer"
	budgetProductAICredits       = "ai_credits"
	budgetProductPremiumRequests = "premium_requests"
	budgetTypeBundlePricing      = "BundlePricing"
)

type Client interface {
	ListBudgets(context.Context, gh.BudgetListRequest) (gh.BudgetListResponse, error)
	GetBudget(context.Context, gh.GetBudgetRequest) (gh.Budget, error)
	GetBudgetUserStates(context.Context, gh.BudgetUserStatesRequest) (gh.BudgetUserStatesResponse, error)
}

type ResolverConfig struct {
	Enterprise string
	Client     Client
	CacheTTL   time.Duration
	Now        func() time.Time
	Logf       func(format string, args ...any)
}

type Resolver struct {
	enterprise string
	client     Client
	cacheTTL   time.Duration
	now        func() time.Time
	logf       func(format string, args ...any)

	mu    sync.Mutex
	cache map[cacheKey]cacheEntry
}

type UserBudget struct {
	Status              string   `json:"status"`
	Source              string   `json:"source,omitempty"`
	BudgetID            string   `json:"budgetId,omitempty"`
	ParentBudgetID      string   `json:"parentBudgetId,omitempty"`
	MonthlyLimitUSD     *float64 `json:"monthlyLimitUsd,omitempty"`
	ConsumedUSD         *float64 `json:"consumedUsd,omitempty"`
	RemainingUSD        *float64 `json:"remainingUsd,omitempty"`
	UsagePercent        *float64 `json:"usagePercent,omitempty"`
	PreventFurtherUsage *bool    `json:"preventFurtherUsage,omitempty"`
}

type cacheKey struct {
	enterprise string
	login      string
}

type cacheEntry struct {
	expiresAt time.Time
	budget    UserBudget
}

func NewResolver(config ResolverConfig) *Resolver {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Resolver{
		enterprise: config.Enterprise,
		client:     config.Client,
		cacheTTL:   config.CacheTTL,
		now:        now,
		logf:       config.Logf,
		cache:      make(map[cacheKey]cacheEntry),
	}
}

func (r *Resolver) ResolveUserBudget(ctx context.Context, login string) (UserBudget, error) {
	if r.client == nil {
		return UserBudget{}, fmt.Errorf("budget client is required")
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return UserBudget{}, fmt.Errorf("GitHub login is required")
	}

	key := cacheKey{enterprise: r.enterprise, login: login}
	if cached, ok := r.lookup(key); ok {
		return cached, nil
	}

	result, err := r.resolveUserBudget(ctx, login)
	if err != nil {
		return UserBudget{}, err
	}
	r.store(key, result)
	return result, nil
}

func (r *Resolver) resolveUserBudget(ctx context.Context, login string) (UserBudget, error) {
	budgets, err := r.listBudgets(ctx, budgetScopeMultiUserCustomer, login)
	if err != nil {
		return UserBudget{}, err
	}

	universal, matchCount, ok := chooseUniversalAICreditBudget(budgets)
	if !ok {
		budgets, err = r.listBudgets(ctx, "", login)
		if err != nil {
			return UserBudget{}, err
		}
		universal, matchCount, ok = chooseUniversalAICreditBudget(budgets)
		if ok && r.logf != nil {
			r.logf("unscoped budget list recovered universal AI credit budget for enterprise %s", r.enterprise)
		}
	}
	if !ok {
		return UserBudget{Status: StatusNotConfigured}, nil
	}
	if matchCount > 1 && r.logf != nil {
		r.logf("multiple universal AI credit budgets found for enterprise %s; using budget %s", r.enterprise, universal.ID)
	}

	userState, err := r.getUserState(ctx, universal.ID, login)
	if err != nil {
		listUserState, ok := userStateFromBudgetList(universal, login)
		if !ok {
			return UserBudget{}, err
		}
		if r.logf != nil {
			r.logf("using budget list user amounts for enterprise %s after user-states lookup failed: %v", r.enterprise, err)
		}
		userState = listUserState
	}

	effective := universal
	source := SourceUniversal
	parentBudgetID := ""
	if overrideID := strings.TrimSpace(userState.OverrideBudgetID); overrideID != "" {
		override, err := r.client.GetBudget(ctx, gh.GetBudgetRequest{
			Enterprise: r.enterprise,
			BudgetID:   overrideID,
		})
		if err != nil {
			return UserBudget{}, err
		}
		effective = override
		source = SourceOverride
		parentBudgetID = universal.ID
	}

	return normalizeUserBudget(effective, userState, source, parentBudgetID), nil
}

func (r *Resolver) listBudgets(ctx context.Context, scope string, login string) ([]gh.Budget, error) {
	var budgets []gh.Budget
	for page := 1; ; page++ {
		response, err := r.client.ListBudgets(ctx, gh.BudgetListRequest{
			Enterprise: r.enterprise,
			Scope:      scope,
			User:       login,
			Page:       page,
			PerPage:    100,
		})
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, response.Budgets...)
		if !response.HasNextPage {
			return budgets, nil
		}
	}
}

func (r *Resolver) getUserState(ctx context.Context, budgetID string, login string) (gh.BudgetUserState, error) {
	response, err := r.client.GetBudgetUserStates(ctx, gh.BudgetUserStatesRequest{
		Enterprise: r.enterprise,
		BudgetID:   budgetID,
		User:       login,
		PerPage:    100,
	})
	if err != nil {
		return gh.BudgetUserState{}, err
	}
	for _, state := range response.UserStates {
		if strings.EqualFold(state.User, login) {
			return state, nil
		}
	}
	if len(response.UserStates) > 0 {
		return response.UserStates[0], nil
	}
	return gh.BudgetUserState{User: login}, nil
}

func userStateFromBudgetList(budget gh.Budget, login string) (gh.BudgetUserState, bool) {
	if budget.ConsumedAmount == nil && budget.TargetAmount == nil && strings.TrimSpace(budget.OverrideBudgetID) == "" {
		return gh.BudgetUserState{}, false
	}

	state := gh.BudgetUserState{
		User:             login,
		OverrideBudgetID: strings.TrimSpace(budget.OverrideBudgetID),
	}
	if budget.ConsumedAmount != nil {
		state.ConsumedAmount = *budget.ConsumedAmount
	}
	if budget.TargetAmount != nil {
		state.TargetAmount = *budget.TargetAmount
	} else {
		state.TargetAmount = budget.BudgetAmount
	}
	return state, true
}

func chooseUniversalAICreditBudget(budgets []gh.Budget) (gh.Budget, int, bool) {
	matches := make([]gh.Budget, 0, len(budgets))
	for _, budget := range budgets {
		if budget.BudgetScope != budgetScopeMultiUserCustomer {
			continue
		}
		if budgetProductTier(budget) < 0 {
			continue
		}
		matches = append(matches, budget)
	}
	if len(matches) == 0 {
		return gh.Budget{}, 0, false
	}

	sort.SliceStable(matches, func(i, j int) bool {
		iTier := budgetSelectionTier(matches[i])
		jTier := budgetSelectionTier(matches[j])
		if iTier != jTier {
			return iTier < jTier
		}
		return matches[i].ID < matches[j].ID
	})
	return matches[0], len(matches), true
}

func budgetSelectionTier(budget gh.Budget) int {
	if budget.BudgetType == budgetTypeBundlePricing && hasBudgetProduct(budget, budgetProductAICredits) {
		return 0
	}
	if hasBudgetProduct(budget, budgetProductAICredits) {
		return 1
	}
	if hasBudgetProduct(budget, budgetProductPremiumRequests) {
		return 2
	}
	return 3
}

func budgetProductTier(budget gh.Budget) int {
	if hasBudgetProduct(budget, budgetProductAICredits) {
		return 0
	}
	if hasBudgetProduct(budget, budgetProductPremiumRequests) {
		return 1
	}
	return -1
}

func hasBudgetProduct(budget gh.Budget, product string) bool {
	if budget.BudgetProductSKU == product {
		return true
	}
	for _, sku := range budget.BudgetProductSKUs {
		if sku == product {
			return true
		}
	}
	return false
}

func normalizeUserBudget(effective gh.Budget, userState gh.BudgetUserState, source string, parentBudgetID string) UserBudget {
	limit := effective.BudgetAmount
	if limit <= 0 {
		limit = userState.TargetAmount
	}
	consumed := userState.ConsumedAmount
	remaining := limit - consumed
	if remaining < 0 {
		remaining = 0
	}

	budget := UserBudget{
		Status:              StatusAvailable,
		Source:              source,
		BudgetID:            effective.ID,
		ParentBudgetID:      parentBudgetID,
		MonthlyLimitUSD:     floatPtr(limit),
		ConsumedUSD:         floatPtr(consumed),
		RemainingUSD:        floatPtr(remaining),
		PreventFurtherUsage: boolPtr(effective.PreventFurtherUsage),
	}
	if limit > 0 {
		budget.UsagePercent = floatPtr((consumed / limit) * 100)
	}
	return budget
}

func (r *Resolver) lookup(key cacheKey) (UserBudget, bool) {
	if r.cacheTTL <= 0 {
		return UserBudget{}, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.cache[key]
	if !ok {
		return UserBudget{}, false
	}
	if !r.now().Before(entry.expiresAt) {
		delete(r.cache, key)
		return UserBudget{}, false
	}
	return cloneUserBudget(entry.budget), true
}

func (r *Resolver) store(key cacheKey, budget UserBudget) {
	if r.cacheTTL <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[key] = cacheEntry{
		expiresAt: r.now().Add(r.cacheTTL),
		budget:    cloneUserBudget(budget),
	}
}

func cloneUserBudget(budget UserBudget) UserBudget {
	if budget.MonthlyLimitUSD != nil {
		budget.MonthlyLimitUSD = floatPtr(*budget.MonthlyLimitUSD)
	}
	if budget.ConsumedUSD != nil {
		budget.ConsumedUSD = floatPtr(*budget.ConsumedUSD)
	}
	if budget.RemainingUSD != nil {
		budget.RemainingUSD = floatPtr(*budget.RemainingUSD)
	}
	if budget.UsagePercent != nil {
		budget.UsagePercent = floatPtr(*budget.UsagePercent)
	}
	if budget.PreventFurtherUsage != nil {
		budget.PreventFurtherUsage = boolPtr(*budget.PreventFurtherUsage)
	}
	return budget
}

func floatPtr(value float64) *float64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
