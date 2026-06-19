# Local Demo Mode Design

## Goal

Allow the dashboard to be viewed locally before Google OAuth and GitHub Enterprise billing credentials are available.

## Design

Local demo mode is explicit and development-only. The web app accepts `AUTH_DEV_EMAIL` outside production and returns a local session for that user. The API accepts `GITHUB_BILLING_FIXTURE_PATH` outside production and serves normalized usage through the same `/v1/usage` endpoint using the existing static identity resolver.

Production safety is enforced in both layers. `AUTH_DEV_EMAIL` is ignored when `NODE_ENV=production`; `GITHUB_BILLING_FIXTURE_PATH` is rejected when `NODE_ENV=production`. Real Google OAuth and live GitHub billing remain the default production path.

## Data Flow

```text
AUTH_DEV_EMAIL
  -> Next.js local session
  -> signed app token
  -> Go API validates token
  -> static email-to-login map
  -> fixture billing client
  -> normalized dashboard response
```

## Testing

Web tests cover dev session creation and the production guard. API tests cover fixture config loading, production rejection, and fixture billing decoding. Existing API, web tests, and builds must keep passing with dummy secrets.
