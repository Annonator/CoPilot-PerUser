# Local Demo Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local-only dashboard demo path that does not require Google OAuth or GitHub Enterprise credentials.

**Architecture:** Add a small dev-session helper in the web app and a fixture billing client in the API. Both are controlled by explicit environment variables and guarded against `NODE_ENV=production`.

**Tech Stack:** Next.js, Auth.js, Vitest, Go HTTP service, Go tests.

---

### Task 1: Web Dev Session

**Files:**
- Create: `src/web/lib/dev-session.ts`
- Create: `src/web/lib/dev-session.test.ts`
- Modify: `src/web/auth.ts`

- [ ] Write failing tests for `AUTH_DEV_EMAIL` session creation and production disablement.
- [ ] Implement `devSessionFromEnv`.
- [ ] Wrap the exported `auth` function so local sessions are returned only when the helper returns a session.
- [ ] Run `npm run test`.

### Task 2: API Fixture Billing

**Files:**
- Modify: `src/api/internal/config/config.go`
- Modify: `src/api/internal/config/config_test.go`
- Create: `src/api/internal/github/fixture_billing.go`
- Create: `src/api/internal/github/fixture_billing_test.go`
- Modify: `src/api/cmd/server/main.go`

- [ ] Write failing tests for `GITHUB_BILLING_FIXTURE_PATH` and the production guard.
- [ ] Write a failing fixture billing client test that decodes `internal/testfixtures/ai-credit-usage.json`.
- [ ] Implement config parsing and the fixture billing client.
- [ ] Select the fixture client in `main.go` when configured.
- [ ] Run `go test ./...`.

### Task 3: Documentation And Verification

**Files:**
- Modify: `.env.example`
- Modify: `docker-compose.yml`
- Modify: `README.md`

- [ ] Document local demo environment variables and commands.
- [ ] Pass demo variables through Docker Compose.
- [ ] Run API and web test/build verification.
