# Agent Notes

This project is a self-service dashboard for GitHub Copilot AI credit usage. Users sign in with Google and see only their own GitHub Enterprise billing AI usage.

## Durable Decisions

- Put application code under `src/`.
- Use `src/web` for the Next.js frontend.
- Use `src/api` for the Go backend service.
- No external auth proxy exists. The frontend owns Google OAuth/OIDC.
- The backend owns all GitHub Enterprise credentials and calls.
- GitHub users are normal GitHub users with SAML SSO, not Enterprise Managed Users.
- The linked GitHub SSO identity `NameID` is the company email address.
- Start with a live proxy plus short cache. Do not persist enterprise-wide billing data initially.
- The app must run both directly on the host and through Docker Compose.
- Store source-backed design specs in `docs/superpowers/specs/`.

## Core Data Flow

```text
Google email
  -> backend validates signed app token/session
  -> backend resolves email to GitHub login through linked SAML identity
  -> backend calls enterprise AI credit billing API filtered by that login
  -> frontend renders normalized self-only usage
```

## GitHub API Target

Use GitHub Enterprise billing AI credit usage:

```text
GET /enterprises/{enterprise}/settings/billing/ai_credit/usage
```

Use the `user`, `year`, and `month` filters for the self-service dashboard. Model/day/org filters can support drill-downs later.

Important: enterprise billing endpoints require enterprise admin or billing-manager credentials and are documented as not supporting GitHub App tokens, installation tokens, or fine-grained PATs at enterprise scope. Keep the credential server-side in `src/api`.

Map billing fields to UI labels consistently:

- `discountQuantity` -> included credits
- `netQuantity` -> additional credits
- `grossAmount` -> gross amount
- `netAmount` -> additional usage
- `pricePerUnit` -> price per credit

## Security Rules

- Never send GitHub admin credentials to `src/web`.
- Never trust an email supplied directly by the browser.
- Derive the GitHub login server-side from the authenticated Google email.
- Normal users may only access their own derived usage.
- Keep admin views, if added later, behind separate endpoints and explicit role checks.
- Do not log OAuth tokens, GitHub admin tokens, or full raw billing payloads.

## Docker Expectations

- Keep service code under `src/`, with `src/web/Dockerfile` and `src/api/Dockerfile`.
- Use a root `docker-compose.yml` for local container startup.
- Compose runs `web` and `api`; do not add a database until persistence is explicitly needed.
- Keep secrets in ignored local env files, not in Compose or Dockerfiles.
- In Compose, the web service reaches the API by service name; direct host development can use `API_BASE_URL`.

## Efficient Workflow

- Use `rg` and `rg --files` first for repo inspection.
- Prefer small interfaces around external systems: auth validation, GitHub billing client, SAML identity resolver, and usage normalization.
- Build fixture-backed local development first, then wire live GitHub credentials.
- Keep the frontend contract stable even if the GitHub identity lookup implementation changes.
- Use focused tests around auth boundaries and self-only authorization before adding UI polish.
