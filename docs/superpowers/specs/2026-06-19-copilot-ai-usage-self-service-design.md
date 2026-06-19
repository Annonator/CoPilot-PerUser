# Copilot AI Usage Self-Service Design

Date: 2026-06-19

## Purpose

Build a self-service web app where a company user signs in with Google and can view only their own GitHub Copilot AI credit usage. The source view is GitHub Enterprise billing AI usage, which currently requires enterprise admin or billing-manager access. The app acts as a narrow server-side proxy that keeps those credentials private and enforces self-only access.

The target user-facing experience mirrors the GitHub Enterprise "AI usage" billing screen for one user: current period totals, daily usage trend, model-level breakdown, included credits, additional credits, gross amount, and additional usage.

## Confirmed Decisions

- Code lives under `src/`.
- Frontend is Next.js.
- There is no external auth proxy.
- Next.js owns Google OAuth/OIDC.
- Go owns all GitHub Enterprise communication and credentials.
- GitHub accounts are normal GitHub users with SAML SSO, not Enterprise Managed Users.
- The linked SSO identity `NameID` is the company email address.
- The app starts as a live proxy with a short cache, not daily ingestion.
- Durable project context is recorded in root `AGENTS.MD`.

## Architecture

```text
Browser
  -> src/web Next.js app
  -> Google OAuth through Auth.js / NextAuth
  -> src/api Go service
  -> GitHub identity resolver: company email -> GitHub login
  -> GitHub Enterprise billing AI credit usage API filtered by login
  -> normalized self-only usage response
```

Recommended repository shape:

```text
src/
  web/
    Next.js app, Google auth, dashboard UI, API client
  api/
    Go HTTP service, auth validation, GitHub clients, usage normalization
  shared/
    Optional generated OpenAPI schema or TypeScript types
docs/
  superpowers/specs/
AGENTS.MD
```

The frontend must never receive or handle GitHub admin credentials. The Go service is the only component allowed to call GitHub billing or identity APIs.

## Authentication And Authorization

The first implementation will use Auth.js / NextAuth in `src/web` with Google as the OAuth provider. The Google sign-in must be restricted by an allowlisted company domain. Use the Google hosted-domain hint for UX, but enforce the domain on the server after login because OAuth request hints are not sufficient as authorization.

The Go API must not trust a raw email string sent by the browser. The initial scaffold will use a signed internal JWT minted by the Next.js server after Auth.js validates the user. The Go API validates that token with `APP_TOKEN_SECRET` and derives the user email from claims.

Authorization is self-only:

- Normal users can request only their own usage.
- The GitHub login used in billing requests is derived server-side from the authenticated email.
- Any future admin view must be a separate endpoint and role check.
- API responses must not include enterprise-wide user lists or raw GitHub billing rows for other users.

## GitHub Integration

Billing source:

- Enterprise endpoint: `GET /enterprises/{enterprise}/settings/billing/ai_credit/usage`
- Required filters for self-service: `user`, `year`, `month`
- Optional filters for drill-down: `day`, `model`, `organization`, `product`
- Relevant fields: `product`, `sku`, `model`, `unitType`, `pricePerUnit`, `grossQuantity`, `grossAmount`, `discountQuantity`, `discountAmount`, `netQuantity`, `netAmount`

GitHub documents that enterprise billing usage endpoints require enterprise admin or billing-manager credentials. Enterprise-scope billing endpoints do not support GitHub App tokens, GitHub App installation tokens, or fine-grained personal access tokens. Store the required credential only in the Go service environment.

Identity source:

- Input: Google-authenticated company email.
- Match: linked GitHub SAML SSO identity `NameID`.
- Output: GitHub login.

The identity resolver should be an interface in Go so the exact lookup mechanism can be swapped without touching usage or HTTP handlers. The production target is GitHub SAML SSO identity data available to enterprise administrators. The scaffold should also include a fixture or static-map implementation for local development and automated tests.

Normalize GitHub billing fields to the screenshot vocabulary:

- `includedCredits`: `discountQuantity`
- `additionalCredits`: `netQuantity`
- `grossAmount`: `grossAmount`
- `additionalUsage`: `netAmount`
- `pricePerCredit`: `pricePerUnit`, expected to be `0.01`

For the daily chart, the backend will make one filtered billing call per day in the selected period, capped to elapsed days for the current month. These daily results share the same cache as the monthly totals.

## Backend API

Initial endpoints:

- `GET /healthz`: process health.
- `GET /v1/me`: authenticated profile returned from validated app token.
- `GET /v1/usage?year=YYYY&month=M`: current user's AI credit usage for a month.

Suggested internal modules:

```text
src/api/cmd/server/
src/api/internal/auth/
src/api/internal/config/
src/api/internal/github/
src/api/internal/identity/
src/api/internal/usage/
src/api/internal/httpapi/
src/api/internal/testfixtures/
```

`usage` should normalize GitHub rows into a UI-oriented shape:

```text
period
user
totals
models[]
daily[]
sourceMetadata
```

The normalized response should preserve enough source metadata for debugging while avoiding raw enterprise-wide data exposure.

## Frontend UI

Initial screens:

- Login page with Google sign-in.
- Authenticated dashboard.
- Period selector for current month and recent months.
- Usage total strip: included credits, additional credits, gross amount, additional usage.
- Daily chart grouped by model.
- Expandable model breakdown table.
- Empty state for no GitHub match or no usage.
- Error state for GitHub auth, permission, and rate-limit failures.

The UI should be a practical dashboard, not a marketing page. It should be optimized for quick scanning by engineers checking their own cost impact.

## Caching And Data Freshness

Start with a short in-memory cache in the Go service:

- Key: enterprise, GitHub login, year, month, optional day/model/organization/product filters.
- TTL: 10 minutes by default, configurable with `USAGE_CACHE_TTL`.
- Purpose: reduce repeated billing API calls while keeping data close to the GitHub billing view.

Do not persist enterprise-wide billing data in the initial scaffold. Add database-backed ingestion only if the live endpoint is too slow, rate-limited, or unavailable for the required UX.

## Configuration

Expected environment variables:

```text
WEB_BASE_URL
GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
AUTH_SECRET
COMPANY_EMAIL_DOMAINS
API_BASE_URL
APP_TOKEN_SECRET

GITHUB_API_BASE_URL
GITHUB_ENTERPRISE_SLUG
GITHUB_ADMIN_TOKEN
GITHUB_IDENTITY_RESOLVER
GITHUB_IDENTITY_STATIC_MAP_PATH
USAGE_CACHE_TTL
```

`GITHUB_API_BASE_URL` defaults to `https://api.github.com` for GitHub Enterprise Cloud. If the enterprise moves to a dedicated `ghe.com` subdomain, configure the dedicated API hostname.

## Error Handling

Return clear, user-safe errors:

- `401`: not signed in or invalid app token.
- `403`: signed-in email is outside the configured company domain.
- `404`: no linked GitHub SAML identity found for the signed-in email.
- `502`: GitHub API failure.
- `503`: GitHub rate limit or temporary outage.

Log operational detail server-side, including GitHub request IDs where available. Do not log full admin tokens, raw OAuth tokens, or full billing payloads.

## Testing

Backend tests:

- Auth token validation accepts only signed expected claims.
- Email domain checks reject non-company users.
- Identity resolver maps email to login and handles no-match.
- GitHub billing client builds correct filtered requests.
- Usage normalization computes totals and model rows from fixtures.
- Authorization tests prove a user cannot request another user's login.

Frontend tests:

- Logged-out users see login.
- Logged-in users see dashboard loading, success, empty, and error states.
- Period selection calls the expected API query.
- Long model/user names do not break table layout.

End-to-end smoke tests can use static identity and usage fixtures before real GitHub credentials exist.

## Open Implementation Risks

The billing endpoint is documented and matches the desired data shape. The identity resolver needs a credential-backed proof against the real enterprise because normal GitHub users with SAML SSO are not SCIM provisioned users. Keep the resolver boundary small so the rest of the scaffold can proceed with a fixture implementation while the exact production lookup is verified.

## Approval

The user approved this design direction on 2026-06-19 before documentation was written.
