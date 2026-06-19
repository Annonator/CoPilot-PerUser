# Enterprise SAML Identity Resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Implement a production resolver that maps authenticated Google company emails to GitHub logins through enterprise-level SAML identity data.

**Architecture:** Add a `github_saml` resolver behind the existing `identity.Resolver` interface. The resolver uses GitHub GraphQL to query `enterprise.ownerInfo.samlIdentityProvider.externalIdentities(first: 2, userName: email, membersOnly: true)`, verifies the returned SAML `nameId`, caches successful email-to-login mappings in memory, and returns typed errors for no-match and duplicate-match cases. Config and server startup select either the existing static resolver or the new live resolver without changing the frontend API contract.

**Tech Stack:** Go standard library HTTP client, GitHub GraphQL over JSON POST, existing Go config/httpapi/usage packages, `go test`.

---

### Task 1: GitHub SAML Resolver Tests

**Files:**
- Modify: `src/api/internal/identity/resolver.go`
- Create: `src/api/internal/identity/github_saml_resolver_test.go`

- [x] **Step 1: Write failing resolver tests**

Add tests that construct an `httptest.Server`, instantiate `NewGitHubSAMLResolver`, and assert:

```go
func TestGitHubSAMLResolverResolvesLogin(t *testing.T)
func TestGitHubSAMLResolverCachesSuccessfulLookup(t *testing.T)
func TestGitHubSAMLResolverReturnsNotFoundForNoMatch(t *testing.T)
func TestGitHubSAMLResolverReturnsNotFoundForMissingSAMLProvider(t *testing.T)
func TestGitHubSAMLResolverReturnsNotFoundForUnclaimedIdentity(t *testing.T)
func TestGitHubSAMLResolverReturnsAmbiguousForDuplicateMatches(t *testing.T)
func TestGitHubSAMLResolverWrapsGraphQLErrors(t *testing.T)
func TestGitHubSAMLResolverDoesNotLeakTokenInHTTPStatusError(t *testing.T)
```

The success test must verify the request method is `POST`, path is `/graphql`,
the `Authorization` header is `Bearer ghp_secret`, variables include
`enterprise=marbis` and a normalized lower-case `email`, and the returned login
is `Annonator`.

- [x] **Step 2: Run tests to verify RED**

Run:

```bash
cd src/api
go test ./internal/identity
```

Expected: fail because `NewGitHubSAMLResolver`, `ErrIdentityNotFound`, and
`ErrIdentityAmbiguous` do not exist yet.

### Task 2: GitHub SAML Resolver Implementation

**Files:**
- Modify: `src/api/internal/identity/resolver.go`
- Create: `src/api/internal/identity/github_saml_resolver.go`

- [x] **Step 1: Add typed identity errors**

In `resolver.go`, add:

```go
var (
	ErrIdentityNotFound  = errors.New("identity not found")
	ErrIdentityAmbiguous = errors.New("identity ambiguous")
)
```

Keep `StaticResolver` behavior but wrap no-match with `ErrIdentityNotFound`:

```go
return "", fmt.Errorf("%w: no GitHub login for email %q", ErrIdentityNotFound, email)
```

- [x] **Step 2: Implement the live resolver**

Create `GitHubSAMLResolver` with:

```go
type GitHubSAMLResolver struct {
	baseURL string
	token string
	enterprise string
	httpClient *http.Client
	cacheTTL time.Duration
	now func() time.Time
	mu sync.Mutex
	cache map[string]githubSAMLCacheEntry
}
```

Expose:

```go
func NewGitHubSAMLResolver(baseURL, token, enterprise string, cacheTTL time.Duration, httpClient *http.Client) *GitHubSAMLResolver
func (r *GitHubSAMLResolver) ResolveGitHubLogin(ctx context.Context, email string) (string, error)
```

The resolver must normalize email with `strings.ToLower(strings.TrimSpace(email))`,
POST JSON to `<baseURL>/graphql`, decode GraphQL `data` and `errors`, reject
missing enterprise/provider/nodes, require exactly one valid matching identity,
require non-empty `user.login`, and cache successful matches until TTL expiry.

- [x] **Step 3: Run tests to verify GREEN**

Run:

```bash
cd src/api
go test ./internal/identity
```

Expected: pass.

### Task 3: Config and HTTP Error Tests

**Files:**
- Modify: `src/api/internal/config/config_test.go`
- Modify: `src/api/internal/httpapi/server_test.go`

- [x] **Step 1: Write failing config tests**

Add tests asserting:

```go
func TestLoadAcceptsGitHubSAMLIdentityResolver(t *testing.T)
func TestLoadDoesNotRequireStaticMapForGitHubSAMLResolver(t *testing.T)
```

The first test must set `GITHUB_IDENTITY_RESOLVER=github_saml` and assert the
value loads. The second must also clear `GITHUB_IDENTITY_STATIC_MAP_PATH` and
assert `Load()` succeeds.

- [x] **Step 2: Write failing HTTP test**

Add a server test where `fakeUsageService.err = identity.ErrIdentityNotFound`
and assert `/v1/usage` returns status `404` and JSON error `not_found`.

- [x] **Step 3: Run tests to verify RED**

Run:

```bash
cd src/api
go test ./internal/config ./internal/httpapi
```

Expected: config tests fail because `github_saml` is unsupported, and HTTP test
fails because all usage errors currently map to `502`.

### Task 4: Config, Server Wiring, and HTTP Mapping

**Files:**
- Modify: `src/api/internal/config/config.go`
- Modify: `src/api/internal/httpapi/server.go`
- Modify: `src/api/cmd/server/main.go`

- [x] **Step 1: Support `github_saml` config**

Allow `GITHUB_IDENTITY_RESOLVER` values `static` and `github_saml`. Require
`GITHUB_IDENTITY_STATIC_MAP_PATH` only when resolver is `static`.

- [x] **Step 2: Map identity no-match to 404**

In `handleUsage`, use `errors.Is(err, identity.ErrIdentityNotFound)` to return:

```go
writeError(w, http.StatusNotFound, "not_found")
```

Keep other usage errors as `502 bad_gateway`.

- [x] **Step 3: Select resolver at startup**

In `cmd/server/main.go`, switch on `cfg.GitHubIdentityResolver`:

```go
case "static":
	resolver, err = identity.NewStaticResolver(cfg.GitHubIdentityStaticMapPath)
case "github_saml":
	resolver = identity.NewGitHubSAMLResolver(cfg.GitHubAPIBaseURL, cfg.GitHubAdminToken, cfg.GitHubEnterpriseSlug, cfg.UsageCacheTTL, http.DefaultClient)
```

- [x] **Step 4: Run tests to verify GREEN**

Run:

```bash
cd src/api
go test ./internal/config ./internal/httpapi ./cmd/server
```

Expected: pass.

### Task 5: Documentation and Full Verification

**Files:**
- Modify: `README.md`

- [x] **Step 1: Document live resolver configuration**

Extend the GitHub Identity Mapping section with:

```env
GITHUB_IDENTITY_RESOLVER=github_saml
GITHUB_ADMIN_TOKEN=enterprise-owner-classic-pat
```

Clarify that `github_saml` needs an enterprise-owner classic PAT with
`read:enterprise` or `admin:enterprise`, while `static` keeps using
`GITHUB_IDENTITY_STATIC_MAP_PATH`.

- [x] **Step 2: Run full backend verification**

Run:

```bash
cd src/api
go test ./...
go build ./cmd/server
```

Expected: both commands pass.

- [x] **Step 3: Review final diff**

Run:

```bash
git diff --stat
git diff -- README.md src/api/internal/identity src/api/internal/config src/api/internal/httpapi src/api/cmd/server/main.go
```

Expected: only resolver, config, HTTP mapping, startup wiring, plan, and README
changes are present.
