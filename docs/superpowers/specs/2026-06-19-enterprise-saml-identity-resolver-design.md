# Enterprise SAML Identity Resolver Design

## Goal

Add a programmatic production resolver that maps an authenticated Google company
email to the linked GitHub login through enterprise-level SAML identity data.
The resolver replaces the current static map for production while keeping the
existing `identity.Resolver` interface and `/v1/usage` contract stable.

## Context

The backend already validates a signed app token from the web service and derives
the user's email from that token. The usage service already depends on
`identity.Resolver`, so the new resolver can be added without changing the
frontend or the billing usage flow.

The enterprise uses enterprise-level SAML SSO. The linked GitHub SAML identity
`NameID` is the company email address. GitHub users are normal GitHub users with
SAML SSO, not Enterprise Managed Users.

## Recommended Approach

Use an on-demand GitHub GraphQL lookup filtered by the normalized email. This
minimizes data exposure because each dashboard request only asks GitHub for the
identity associated with the authenticated user.

The static resolver remains available for local development, fixtures, and
tests. Production can opt into the live resolver with:

```env
GITHUB_IDENTITY_RESOLVER=github_saml
```

## GitHub GraphQL Query

The resolver calls GitHub GraphQL through the configured API base URL and
enterprise slug:

```graphql
query ResolveGitHubLogin($enterprise: String!, $email: String!) {
  enterprise(slug: $enterprise) {
    ownerInfo {
      samlIdentityProvider {
        externalIdentities(first: 2, userName: $email, membersOnly: true) {
          nodes {
            samlIdentity {
              nameId
            }
            user {
              login
            }
          }
        }
      }
    }
  }
}
```

`first: 2` lets the resolver detect duplicate identities without fetching more
data than needed. `membersOnly: true` restricts matches to identities with valid
membership.

The credential must be able to read enterprise owner information and SAML
identity provider data. GitHub's schema describes `ownerInfo` as visible to
enterprise owners or classic personal access tokens with `read:enterprise` or
`admin:enterprise`. A billing-manager-only token may be sufficient for billing
usage but not for identity resolution.

## Data Flow

```text
Google email from validated app token
  -> normalize email with trim + lower-case
  -> GitHub GraphQL enterprise SAML lookup by userName/NameID
  -> verify returned samlIdentity.nameId matches the normalized email
  -> return returned user.login
  -> existing usage service calls billing endpoint filtered by login
```

The IdP should emit lower-case company email addresses as SAML NameID values.
The resolver will verify returned NameID values case-insensitively after
normalizing, but the GitHub `userName` filter may depend on the canonical value
stored by GitHub.

## Components

- `identity.StaticResolver`: unchanged, still selected by `static`.
- `identity.GitHubSAMLResolver`: new resolver implementing
  `ResolveGitHubLogin(ctx, email)`.
- `identity.GraphQLClient`: small internal HTTP client for GitHub GraphQL POSTs.
- `config.Config`: allow `GITHUB_IDENTITY_RESOLVER=github_saml`; keep static map
  path required only for `static`.
- `cmd/server`: select the resolver from config.
- `httpapi.Server`: map no-match identity errors to `404`.

No new database is required. The resolver remains a live proxy with a short
cache, matching the project decision not to persist enterprise-wide billing or
identity data initially.

## Caching

Cache successful `email -> githubLogin` resolutions in memory using the existing
`UsageCacheTTL` value for the first implementation. The current default of 10
minutes is enough to reduce repeated GraphQL calls without making identity
changes stale for long.

Do not cache errors at first. No-match and permission failures should recover as
soon as GitHub or configuration is fixed.

## Error Handling

The resolver returns typed errors:

- `ErrIdentityNotFound`: no enterprise, no SAML provider, no matching identity,
  unclaimed identity, or empty login.
- `ErrIdentityAmbiguous`: more than one identity matches the same NameID.
- wrapped upstream errors for HTTP failures, GraphQL errors, malformed responses,
  and permission failures.

The HTTP layer should return:

- `404` for `ErrIdentityNotFound`.
- `502` for ambiguous identity, GitHub API failures, malformed responses, and
  permission failures.

Responses must remain generic. Server logs can include high-level context such
as the normalized email hash or domain and GitHub request ID, but must not log
OAuth tokens, GitHub tokens, or full raw identity payloads.

## Security

The browser never receives GitHub credentials or raw identity data. The backend
never trusts an email supplied in query parameters or request bodies. It only
uses the email from the validated app token.

Normal users can only trigger lookup for their own authenticated email because
`/v1/usage` ignores browser-supplied `user` and `email` query values.

## Testing

Backend tests should cover:

- config accepts `github_saml` and rejects missing static map only for `static`.
- the resolver sends the expected GraphQL query variables.
- successful lookup returns `user.login`.
- case/whitespace email normalization.
- no match returns `ErrIdentityNotFound`.
- missing SAML provider returns `ErrIdentityNotFound`.
- unclaimed identity with nil `user` returns `ErrIdentityNotFound`.
- duplicate matches return `ErrIdentityAmbiguous`.
- GraphQL errors and non-2xx HTTP statuses are wrapped without leaking tokens.
- `/v1/usage` maps identity no-match to `404`.

Existing static resolver tests remain in place for fixture and local demo mode.

## Rollout

1. Keep current static resolver as the default for local development.
2. Implement `github_saml` behind the existing resolver interface.
3. Run backend tests with fixture HTTP servers.
4. Test against the real enterprise with an enterprise-owner classic PAT that
   can read SAML identity provider data.
5. Switch production config to `GITHUB_IDENTITY_RESOLVER=github_saml`.

## Sources

- GitHub GraphQL public schema: `Query.enterprise`, `Enterprise.ownerInfo`,
  `EnterpriseOwnerInfo.samlIdentityProvider`,
  `EnterpriseIdentityProvider.externalIdentities`, `ExternalIdentity.user`, and
  `ExternalIdentitySamlAttributes.nameId`.
- GitHub GraphQL documentation for POST requests and variables.
- GitHub GraphQL pagination documentation for connection limits and cursors.
