package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const gitHubSAMLResolveQuery = `
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
}`

type GitHubSAMLResolver struct {
	baseURL    string
	token      string
	enterprise string
	httpClient *http.Client
	cacheTTL   time.Duration
	now        func() time.Time

	mu    sync.Mutex
	cache map[string]githubSAMLCacheEntry
}

type githubSAMLCacheEntry struct {
	expiresAt time.Time
	login     string
}

func NewGitHubSAMLResolver(baseURL, token, enterprise string, cacheTTL time.Duration, httpClient *http.Client) *GitHubSAMLResolver {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &GitHubSAMLResolver{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		enterprise: enterprise,
		httpClient: httpClient,
		cacheTTL:   cacheTTL,
		now:        time.Now,
		cache:      make(map[string]githubSAMLCacheEntry),
	}
}

func (r *GitHubSAMLResolver) ResolveGitHubLogin(ctx context.Context, email string) (string, error) {
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return "", fmt.Errorf("%w: empty email", ErrIdentityNotFound)
	}
	if login, ok := r.cached(normalizedEmail); ok {
		return login, nil
	}

	response, err := r.queryExternalIdentity(ctx, normalizedEmail)
	if err != nil {
		return "", err
	}
	login, err := resolveLoginFromGraphQL(response, normalizedEmail)
	if err != nil {
		return "", err
	}
	r.store(normalizedEmail, login)
	return login, nil
}

func (r *GitHubSAMLResolver) queryExternalIdentity(ctx context.Context, email string) (githubSAMLGraphQLResponse, error) {
	body := githubSAMLGraphQLRequest{
		Query: gitHubSAMLResolveQuery,
		Variables: map[string]string{
			"enterprise": r.enterprise,
			"email":      email,
		},
	}
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(body); err != nil {
		return githubSAMLGraphQLResponse{}, fmt.Errorf("encode GitHub GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/graphql", &encoded)
	if err != nil {
		return githubSAMLGraphQLResponse{}, fmt.Errorf("create GitHub GraphQL request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return githubSAMLGraphQLResponse{}, fmt.Errorf("request GitHub GraphQL identity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return githubSAMLGraphQLResponse{}, fmt.Errorf("GitHub GraphQL identity status %d", resp.StatusCode)
	}

	var graphQLResponse githubSAMLGraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&graphQLResponse); err != nil {
		return githubSAMLGraphQLResponse{}, fmt.Errorf("decode GitHub GraphQL identity: %w", err)
	}
	if len(graphQLResponse.Errors) > 0 {
		return githubSAMLGraphQLResponse{}, fmt.Errorf("GitHub GraphQL identity error: %s", graphQLResponse.Errors[0].Message)
	}
	return graphQLResponse, nil
}

func resolveLoginFromGraphQL(response githubSAMLGraphQLResponse, email string) (string, error) {
	if response.Data.Enterprise == nil {
		return "", fmt.Errorf("%w: enterprise not found", ErrIdentityNotFound)
	}
	if response.Data.Enterprise.OwnerInfo == nil {
		return "", fmt.Errorf("%w: enterprise owner info unavailable", ErrIdentityNotFound)
	}
	provider := response.Data.Enterprise.OwnerInfo.SAMLIdentityProvider
	if provider == nil {
		return "", fmt.Errorf("%w: SAML identity provider not found", ErrIdentityNotFound)
	}

	var matches []githubSAMLExternalIdentity
	for _, node := range provider.ExternalIdentities.Nodes {
		if node.SAMLIdentity == nil {
			continue
		}
		if normalizeEmail(node.SAMLIdentity.NameID) != email {
			continue
		}
		matches = append(matches, node)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("%w: no SAML identity for email", ErrIdentityNotFound)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%w: multiple SAML identities for email", ErrIdentityAmbiguous)
	}

	match := matches[0]
	if match.User == nil || strings.TrimSpace(match.User.Login) == "" {
		return "", fmt.Errorf("%w: SAML identity is not linked to a GitHub user", ErrIdentityNotFound)
	}
	return strings.TrimSpace(match.User.Login), nil
}

func (r *GitHubSAMLResolver) cached(email string) (string, bool) {
	if r.cacheTTL <= 0 {
		return "", false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.cache[email]
	if !ok {
		return "", false
	}
	if !r.now().Before(entry.expiresAt) {
		delete(r.cache, email)
		return "", false
	}
	return entry.login, true
}

func (r *GitHubSAMLResolver) store(email, login string) {
	if r.cacheTTL <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[email] = githubSAMLCacheEntry{
		expiresAt: r.now().Add(r.cacheTTL),
		login:     login,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

type githubSAMLGraphQLRequest struct {
	Query     string            `json:"query"`
	Variables map[string]string `json:"variables"`
}

type githubSAMLGraphQLResponse struct {
	Data   githubSAMLGraphQLData    `json:"data"`
	Errors []githubSAMLGraphQLError `json:"errors"`
}

type githubSAMLGraphQLData struct {
	Enterprise *githubSAMLEnterprise `json:"enterprise"`
}

type githubSAMLEnterprise struct {
	OwnerInfo *githubSAMLOwnerInfo `json:"ownerInfo"`
}

type githubSAMLOwnerInfo struct {
	SAMLIdentityProvider *githubSAMLIdentityProvider `json:"samlIdentityProvider"`
}

type githubSAMLIdentityProvider struct {
	ExternalIdentities githubSAMLExternalIdentityConnection `json:"externalIdentities"`
}

type githubSAMLExternalIdentityConnection struct {
	Nodes []githubSAMLExternalIdentity `json:"nodes"`
}

type githubSAMLExternalIdentity struct {
	SAMLIdentity *githubSAMLIdentityAttributes `json:"samlIdentity"`
	User         *githubSAMLUser               `json:"user"`
}

type githubSAMLIdentityAttributes struct {
	NameID string `json:"nameId"`
}

type githubSAMLUser struct {
	Login string `json:"login"`
}

type githubSAMLGraphQLError struct {
	Message string `json:"message"`
}
