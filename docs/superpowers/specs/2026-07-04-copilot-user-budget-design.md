# Copilot User Budget Resolution Design

Date: 2026-07-04

## Purpose

Add user-facing Copilot AI credit budget information to the self-service usage dashboard without exposing GitHub Enterprise billing credentials to the frontend.

The budget feature should answer: "What budget currently applies to this signed-in GitHub user?" The first implementation only considers universal per-user AI credit budgets and explicit per-user overrides. Enterprise, organization, repository, cost center, and general spend budgets are out of scope.

## Source Context

Current usage flow:

```text
Google-authenticated email
  -> Go API validates signed app token
  -> Go API resolves email to GitHub login
  -> Go API fetches GitHub Enterprise AI credit usage filtered by login
  -> frontend renders normalized self-only usage
```

Existing relevant files:

```text
src/api/internal/github/billing.go
src/api/internal/usage/service.go
src/api/internal/usage/models.go
src/api/internal/httpapi/server.go
src/web/lib/usage-types.ts
src/web/components/usage-dashboard.tsx
```

`MonthlyUsage` already has an optional `budget` field, but it is currently backed by a static configured value. This should become a GitHub-backed effective budget result. The old static value can remain only as a local development fallback if useful.

## GitHub Budget APIs

Use GitHub Enterprise Cloud billing budget endpoints from the backend only:

```text
GET /enterprises/{enterprise}/settings/billing/budgets?scope=multi_user_customer
GET /enterprises/{enterprise}/settings/billing/budgets/{budget_id}/user-states?user={github_login}
GET /enterprises/{enterprise}/settings/billing/budgets/{budget_id}
```

Budget endpoints require enterprise admin or billing-manager credentials. GitHub documents that enterprise budget endpoints do not work with GitHub App user tokens, GitHub App installation tokens, or fine-grained personal access tokens.

The implementation should use the existing server-side `GITHUB_ADMIN_TOKEN` pattern and must never send budget API credentials or raw enterprise budget lists to `src/web`.

## Effective Budget Algorithm

After resolving the authenticated user's GitHub login:

1. Fetch enterprise budgets with `scope=multi_user_customer`.
2. Filter to Copilot AI credit budgets:
   - `budget_product_sku == "ai_credits"` or `budget_product_skus` contains `"ai_credits"`.
   - Prefer `budget_type == "BundlePricing"` when multiple budget records match.
3. For the selected universal budget, call:

   ```text
   GET /enterprises/{enterprise}/settings/billing/budgets/{budget_id}/user-states?user={github_login}
   ```

4. If the returned user state has `override_budget_id`, fetch that budget by ID and treat it as the effective budget.
5. If there is no override, use the universal budget and the returned user state as the effective budget.
6. If no universal AI credit budget exists, return `budget.status = "not_configured"`.

When multiple universal AI credit budgets exist, choose a deterministic order and log an operational warning. Recommended priority:

1. `BundlePricing` plus `ai_credits`
2. `ai_credits` match over legacy `premium_requests`
3. Lowest stable sort by budget ID as the final tie-breaker

Do not include enterprise, organization, repository, or cost-center spend budgets in this calculation.

## Backend Design

Add raw GitHub budget types and methods under `src/api/internal/github`. Keep them separate from usage normalization.

Recommended raw client methods:

```go
type BudgetClient interface {
    ListBudgets(ctx context.Context, req BudgetListRequest) (BudgetListResponse, error)
    GetBudget(ctx context.Context, req GetBudgetRequest) (Budget, error)
    GetBudgetUserStates(ctx context.Context, req BudgetUserStatesRequest) (BudgetUserStatesResponse, error)
}
```

Add a higher-level resolver in `src/api/internal/usage` or a new `src/api/internal/budget` package:

```go
type BudgetResolver interface {
    ResolveUserBudget(ctx context.Context, login string) (UserBudget, error)
}
```

The usage service should call the resolver after it has resolved the GitHub login. Budget lookup is supplemental to usage lookup.

## API Contract

Replace the current narrow `monthlyIncludedCredits` budget shape with a status-based shape:

```json
{
  "budget": {
    "status": "available",
    "source": "override",
    "budgetId": "override-budget-id",
    "parentBudgetId": "universal-budget-id",
    "monthlyLimitUsd": 30,
    "consumedUsd": 12.5,
    "remainingUsd": 17.5,
    "usagePercent": 41.7,
    "preventFurtherUsage": true
  }
}
```

Allowed statuses:

```text
available       A universal or override user budget was resolved.
not_configured  No universal AI credit budget applies.
unavailable     GitHub budget lookup failed, but usage data is still available.
```

Allowed sources when `status = "available"`:

```text
universal  The enterprise universal per-user AI credit budget applies.
override   A per-user override budget applies.
```

Field semantics:

- `monthlyLimitUsd`: the effective budget amount in whole USD from the budget API, or `target_amount` when that is the only available effective target.
- `consumedUsd`: per-user consumed amount from `user-states`.
- `remainingUsd`: `max(monthlyLimitUsd - consumedUsd, 0)`.
- `usagePercent`: `(consumedUsd / monthlyLimitUsd) * 100`, omitted or zero when the limit is zero.
- `preventFurtherUsage`: copied from the effective budget.
- `parentBudgetId`: set only when an override budget applies.

Do not call this "included credits" in the contract or UI. Existing usage fields keep their current meaning:

- `includedCredits` comes from GitHub billing usage `discountQuantity`.
- `additionalCredits` comes from GitHub billing usage `netQuantity`.
- budget amount is a dollar-denominated control from the budget API.

## Failure Behavior

Usage should remain the primary response. If usage succeeds but budget lookup fails, return HTTP 200 with:

```json
{
  "budget": {
    "status": "unavailable"
  }
}
```

Only fail the whole `/v1/usage` request when the core self-only usage lookup fails.

Do not log raw budget payloads. Logs may include request path category, enterprise slug, GitHub login, status code, and GitHub request ID if available.

## Caching

Cache effective budget results separately from usage:

```text
key: enterprise + githubLogin
ttl: USAGE_CACHE_TTL, default 10 minutes
```

Budget state is not period-filtered in the documented API. Treat the budget result as the current effective budget. If the frontend is displaying historical months, label the budget as current or omit it from historical views to avoid implying historical budget accuracy.

## Frontend Design

Display budget as a separate concept from usage totals:

```text
Monthly budget
Used
Remaining
Budget source: Universal / Override
```

For `not_configured`, show a quiet "No user budget configured" state.

For `unavailable`, show "Budget unavailable" without blocking the rest of the usage dashboard.

Do not expose raw budget IDs by default in the UI. Keep IDs in the API response for debugging and tests, but render them only in developer-oriented metadata if needed later.

## Tests

Backend tests should cover:

- Resolves universal budget when no override exists.
- Resolves override budget when `override_budget_id` exists.
- Ignores enterprise, organization, repository, cost center, and non-AI-credit budgets.
- Handles no universal budget as `not_configured`.
- Handles budget API failure as `unavailable` without failing usage.
- Caches effective budget by enterprise and login.
- Does not use browser-supplied email or login for budget lookup.

Frontend tests should cover:

- Available universal budget display.
- Available override budget display.
- Not-configured state.
- Unavailable state.
- Budget values are not confused with included credit usage totals.

## Open Implementation Notes

- Decide whether the current `COPILOT_MONTHLY_INCLUDED_CREDITS` env var should be removed, renamed for fixture-only usage, or kept only for local fallback.
- Add fixture budget data for local demo mode if the UI should show a budget without live GitHub credentials.
- Consider a separate `sourceMetadata.budgetCached` field only if the UI needs to distinguish usage cache from budget cache.
