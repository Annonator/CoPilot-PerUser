# CoPilot-PerUser

Self-service dashboard for GitHub Copilot AI credit usage.

Users authenticate with Google. The Go API resolves the Google email to a GitHub Enterprise SAML-linked login and returns only that user's AI credit billing usage.

## Project Layout

```text
src/web   Next.js frontend
src/api   Go backend
docs      specs and implementation plans
```

## Local Configuration

Create local environment files from `.env.example`.

```bash
cp .env.example .env
cp .env.example src/web/.env.local
```

Generate `AUTH_SECRET` and `APP_TOKEN_SECRET` with `openssl rand -base64 32`
and store them in ignored env files or shell environment. Docker Compose does
not provide fallback values for these secrets. Placeholder values and short
strings are rejected at startup/runtime.

Never commit real Google OAuth secrets, GitHub enterprise admin tokens,
`AUTH_SECRET`, or `APP_TOKEN_SECRET`.

## Local Demo Without Google or GitHub

Use local demo mode to view the real frontend before configuring Google OAuth
or a GitHub Enterprise billing credential. Demo mode is disabled when
`NODE_ENV=production`.

For host development, put the web values in `src/web/.env.local`:

```env
AUTH_SECRET=replace-with-output-of-openssl-rand-base64-32
AUTH_GOOGLE_ID=unused
AUTH_GOOGLE_SECRET=unused
AUTH_DEV_EMAIL=user@company.name
AUTH_DEV_NAME=Local Demo User
COMPANY_EMAIL_DOMAINS=company.name
APP_TOKEN_SECRET=replace-with-output-of-openssl-rand-base64-32
API_BASE_URL=http://localhost:8080
```

Replace both secret placeholders with generated values. The API command below
must use the same `APP_TOKEN_SECRET` value that you put in `src/web/.env.local`;
export it in the API terminal before starting the service.

Then run the API and web app in separate terminals:

```bash
cd src/api
PORT=8080 \
COMPANY_EMAIL_DOMAINS=company.name \
APP_TOKEN_SECRET="$APP_TOKEN_SECRET" \
GITHUB_ENTERPRISE_SLUG=local-demo \
GITHUB_ADMIN_TOKEN= \
GITHUB_IDENTITY_RESOLVER=static \
GITHUB_IDENTITY_STATIC_MAP_PATH=internal/testfixtures/identity-map.json \
GITHUB_BILLING_FIXTURE_PATH=internal/testfixtures/ai-credit-usage.json \
go run ./cmd/server
```

```bash
cd src/web
npm install
npm run dev
```

Open `http://localhost:3000`. The dashboard signs in as
`user@company.name`, maps that email to the fixture GitHub login, and renders
fixture AI credit usage through the normal `/v1/usage` API path.

For Docker Compose demo mode, use the container fixture path:

```bash
AUTH_DEV_EMAIL=user@company.name \
AUTH_DEV_NAME="Local Demo User" \
APP_TOKEN_SECRET="$(openssl rand -base64 32)" \
AUTH_SECRET="$(openssl rand -base64 32)" \
COMPANY_EMAIL_DOMAINS=company.name \
GITHUB_ENTERPRISE_SLUG=local-demo \
GITHUB_ADMIN_TOKEN= \
GITHUB_BILLING_FIXTURE_PATH=/app/config/ai-credit-usage.json \
docker compose up --build
```

## Enterprise Configuration

The app needs configuration in Google Workspace / Google Cloud, GitHub Enterprise,
and local environment files before real users can sign in and see usage.

### Google OAuth

Create a Google OAuth client in Google Cloud Console:

- Application type: `Web application`
- Authorized JavaScript origins:
  - `http://localhost:3000`
  - `https://your-production-domain`
- Authorized redirect URIs:
  - `http://localhost:3000/api/auth/callback/google`
  - `https://your-production-domain/api/auth/callback/google`

For a company-only deployment, configure the OAuth consent screen as an internal
Google Workspace app. If the app is external or in testing mode, add the intended
users as test users.

Set the generated OAuth values in `.env` and, for host web development, in
`src/web/.env.local`:

```env
AUTH_GOOGLE_ID=your-google-client-id.apps.googleusercontent.com
AUTH_GOOGLE_SECRET=your-google-client-secret
AUTH_SECRET=replace-with-output-of-openssl-rand-base64-32
COMPANY_EMAIL_DOMAINS=your-company.com
WEB_BASE_URL=http://localhost:3000
```

`AUTH_GOOGLE_ID` must be the OAuth web client ID. If it is left as
`replace-with-google-client-id`, Google will reject sign-in with
`Error 401: invalid_client`.

### GitHub Enterprise Billing Access

The API service calls GitHub Enterprise billing endpoints server-side. Use a
GitHub credential with enterprise admin or billing-manager access, and never
expose it to `src/web`.

GitHub Enterprise billing usage endpoints require an enterprise admin or
billing-manager credential. Enterprise-scope billing endpoints are documented as
not supporting GitHub App tokens, GitHub App installation tokens, or fine-grained
personal access tokens.

```env
GITHUB_API_BASE_URL=https://api.github.com
GITHUB_ENTERPRISE_SLUG=your-enterprise-slug
GITHUB_ADMIN_TOKEN=your-enterprise-billing-token
```

The enterprise slug is the value from GitHub Enterprise URLs such as
`https://github.com/enterprises/YOUR-SLUG`.

### Internal App Token

`APP_TOKEN_SECRET` is a private shared secret used only between the Next.js web
service and the Go API service. After Google sign-in, the web service signs a
short-lived internal JWT containing the verified user email. The API validates
that JWT before using the email to resolve the user's GitHub login.

This is not a Google OAuth secret and not a GitHub token. Both services must use
the same value, and it should be a long random string:

```env
APP_TOKEN_SECRET=replace-with-output-of-openssl-rand-base64-32
```

### Optional Cloud Run IAM for the API

By default, the web service calls the API with only the internal app JWT in the
standard `Authorization` header. This keeps host development, Docker Compose,
and non-GCP deployments portable.

When deploying the API as a private Cloud Run service, set this web-only
environment variable:

```env
API_ID_TOKEN_AUDIENCE=https://your-api-service-xyz.a.run.app
```

When `API_ID_TOKEN_AUDIENCE` is set, the web service fetches a Google-signed ID
token from the Cloud Run metadata server and sends it as
`X-Serverless-Authorization`. The existing internal app JWT remains in
`Authorization`, so the Go API continues to derive the user email from the
signed app token.

For Cloud Run, the audience must be the receiving API service URL, not a custom
domain or a path-specific URL. Leave `API_ID_TOKEN_AUDIENCE` empty outside
Cloud Run; the metadata server is not available on a local machine.

Example hardening commands:

```bash
gcloud run deploy copilot-usage-api \
  --no-allow-unauthenticated

gcloud run services add-iam-policy-binding copilot-usage-api \
  --member="serviceAccount:copilot-usage-web@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/run.invoker"

gcloud run services update copilot-usage-web \
  --update-env-vars "API_ID_TOKEN_AUDIENCE=https://your-api-service-xyz.a.run.app"
```

### GitHub Identity Mapping

The app must map each authenticated company email address to a GitHub login
before it can request self-only billing usage.

For local development, fixtures, or manually maintained mappings, use the
static resolver:

```env
GITHUB_IDENTITY_RESOLVER=static
GITHUB_IDENTITY_STATIC_MAP_PATH=src/api/internal/testfixtures/identity-map.json
```

The map file should contain lower-case email keys and GitHub login values:

```json
{
  "person@your-company.com": "github-login"
}
```

In Docker Compose, the static map is mounted into the API container at
`/app/config/identity-map.json`.

For production enterprise-level SAML lookup, use the live resolver:

```env
GITHUB_IDENTITY_RESOLVER=github_saml
GITHUB_ADMIN_TOKEN=enterprise-owner-classic-pat
```

The `github_saml` resolver maps the Google email to the GitHub login through
the enterprise-level SAML `NameID`. It requires a credential that can read
enterprise owner information and SAML identity provider data. Use an
enterprise-owner classic personal access token with `read:enterprise` or
`admin:enterprise`. A billing-manager-only credential may be sufficient for
billing usage but not for identity lookup. If one `GITHUB_ADMIN_TOKEN` is used
for both billing and identity resolution, it must satisfy both permission
requirements.

### Cache TTL

The API caches GitHub billing usage responses and successful
`email -> GitHub login` SAML identity lookups in memory for 10 minutes by
default. Configure the shared TTL with Go duration syntax:

```env
USAGE_CACHE_TTL=10m
```

Examples: `5m`, `30m`, `1h`. Failed identity lookups are not cached, so fixed
GitHub membership, SAML, or token configuration is retried on the next request.

### Usage Reporting Window

`GET /v1/usage` accepts only recent, non-future monthly periods. The default
window is the current month plus the previous five months. Configure a different
positive month count when needed:

```env
USAGE_REPORTING_WINDOW_MONTHS=6
```

Requests outside the configured window return HTTP `400` with JSON error
`period_out_of_range`. Malformed periods and future periods return HTTP `400`
with JSON error `bad_request`.

The default monthly usage response includes monthly totals and model breakdowns.
The `daily` field is returned as an empty array until a dedicated drill-down API
is added.

## Host Development

Run the API:

```bash
cd src/api
go test ./...
go run ./cmd/server
```

Run the web app:

```bash
cd src/web
npm install
npm run dev
```

Open `http://localhost:3000`.

For a local production build without real OAuth credentials:

```bash
cd src/web
AUTH_SECRET="$(openssl rand -base64 32)" APP_TOKEN_SECRET="$(openssl rand -base64 32)" COMPANY_EMAIL_DOMAINS=company.name API_BASE_URL=http://localhost:8080 AUTH_GOOGLE_ID=test AUTH_GOOGLE_SECRET=test npm run build
```

## Docker Development

```bash
docker compose up --build
```

The web service runs on `http://localhost:3000`. The API service runs on `http://localhost:8080`.

Validate the API from the host:

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```
