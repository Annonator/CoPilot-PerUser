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

Never commit real Google OAuth secrets or GitHub enterprise admin tokens.

## Local Demo Without Google or GitHub

Use local demo mode to view the real frontend before configuring Google OAuth
or a GitHub Enterprise billing credential. Demo mode is disabled when
`NODE_ENV=production`.

For host development, put the web values in `src/web/.env.local`:

```env
AUTH_SECRET=local-auth-secret
AUTH_GOOGLE_ID=unused
AUTH_GOOGLE_SECRET=unused
AUTH_DEV_EMAIL=user@company.name
AUTH_DEV_NAME=Local Demo User
COMPANY_EMAIL_DOMAINS=company.name
APP_TOKEN_SECRET=local-app-token-secret
API_BASE_URL=http://localhost:8080
```

Then run the API and web app in separate terminals:

```bash
cd src/api
PORT=8080 \
COMPANY_EMAIL_DOMAINS=company.name \
APP_TOKEN_SECRET=local-app-token-secret \
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
APP_TOKEN_SECRET=local-app-token-secret \
AUTH_SECRET=local-auth-secret \
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
AUTH_SECRET=replace-with-a-long-random-secret
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
APP_TOKEN_SECRET=replace-with-another-long-random-secret
```

### GitHub Identity Mapping

The app must map each authenticated company email address to a GitHub login
before it can request self-only billing usage. The current implementation
requires the static resolver because no live SAML identity resolver is wired yet.
Configure it with:

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
AUTH_SECRET=test APP_TOKEN_SECRET=test COMPANY_EMAIL_DOMAINS=company.name API_BASE_URL=http://localhost:8080 AUTH_GOOGLE_ID=test AUTH_GOOGLE_SECRET=test npm run build
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
