package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const gitHubSAMLResolveQuery = `
query ResolveGitHubLogin($enterprise: String!, $email: String!) {
  enterprise(slug: $enterprise) {
    ownerInfo {
      samlIdentityProvider {
        externalIdentities(first: 2, userName: $email, membersOnly: false) {
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

// These limits are per resolver/process. Replicas sharing a GitHub credential
// must account for their combined request rate when sizing a deployment.
const (
	githubSAMLNegativeTTL      = 30 * time.Second
	githubSAMLBackoff          = 5 * time.Second
	githubSAMLMaxBackoff       = time.Minute
	githubSAMLRequestInterval  = time.Second
	githubSAMLOperationTimeout = 10 * time.Second
	githubSAMLMaxConcurrent    = 4
	githubSAMLMaxCacheEntries  = 1024
	githubSAMLMaxResponseBytes = 64 << 10
)

var errGitHubSAMLBusy = errors.New("GitHub GraphQL identity temporarily unavailable")

type GitHubSAMLResolver struct {
	baseURL          string
	token            string
	enterprise       string
	httpClient       *http.Client
	cacheTTL         time.Duration
	now              func() time.Time
	operationTimeout time.Duration

	mu            sync.Mutex
	cache         map[[sha256.Size]byte]githubSAMLCacheEntry
	inflight      map[[sha256.Size]byte]*githubSAMLCall
	nextRequestAt time.Time
	failures      uint
}

type githubSAMLCacheEntry struct {
	expiresAt time.Time
	login     string
	err       error
}

type githubSAMLCall struct {
	done  chan struct{}
	login string
	err   error
}

func NewGitHubSAMLResolver(baseURL, token, enterprise string, cacheTTL time.Duration, httpClient *http.Client) *GitHubSAMLResolver {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &GitHubSAMLResolver{
		baseURL:          strings.TrimRight(baseURL, "/"),
		token:            token,
		enterprise:       enterprise,
		httpClient:       httpClient,
		cacheTTL:         cacheTTL,
		now:              time.Now,
		operationTimeout: githubSAMLOperationTimeout,
		cache:            make(map[[sha256.Size]byte]githubSAMLCacheEntry),
		inflight:         make(map[[sha256.Size]byte]*githubSAMLCall),
	}
}

func (r *GitHubSAMLResolver) ResolveGitHubLogin(ctx context.Context, email string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return "", fmt.Errorf("%w: empty email", ErrIdentityNotFound)
	}
	// Fixed-size keys bound retained key memory without changing identity syntax.
	key := sha256.Sum256([]byte(normalizedEmail))
	r.mu.Lock()
	now := r.now()
	if entry, ok := r.cache[key]; ok {
		if now.Before(entry.expiresAt) {
			r.mu.Unlock()
			return entry.login, entry.err
		}
		delete(r.cache, key)
	}
	call, exists := r.inflight[key]
	if !exists {
		// Reject excess distinct identities without retaining a queue or starting
		// goroutines. Cached identities and existing waiters bypass this admission.
		if len(r.inflight) >= githubSAMLMaxConcurrent || now.Before(r.nextRequestAt) {
			r.mu.Unlock()
			return "", errGitHubSAMLBusy
		}
		call = &githubSAMLCall{done: make(chan struct{})}
		r.inflight[key] = call
		r.nextRequestAt = now.Add(githubSAMLRequestInterval)
		go r.resolve(key, normalizedEmail, call)
	}
	r.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-call.done:
		return call.login, call.err
	}
}

func (r *GitHubSAMLResolver) resolve(key [sha256.Size]byte, email string, call *githubSAMLCall) {
	// A caller disconnecting must not cancel a shared lookup and enable another
	// quota-consuming attempt. This work is independently bounded by a deadline.
	ctx, cancel := context.WithTimeout(context.Background(), r.operationTimeout)
	defer cancel()
	response, err := r.queryExternalIdentity(ctx, email)
	var login string
	if err == nil {
		login, err = resolveLoginFromGraphQL(response, email)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	ttl := r.cacheTTL
	if errors.Is(err, ErrIdentityNotFound) || errors.Is(err, ErrIdentityAmbiguous) {
		ttl = githubSAMLNegativeTTL
		r.failures = 0
	} else if err != nil {
		if r.failures < 5 {
			r.failures++
		}
		ttl = min(githubSAMLBackoff<<(r.failures-1), githubSAMLMaxBackoff)
		var statusErr *githubSAMLStatusError
		if errors.As(err, &statusErr) {
			ttl = max(ttl, statusErr.retryAfter)
		}
		// A global cooldown also protects quota when callers vary their identities.
		r.nextRequestAt = maxTime(r.nextRequestAt, now.Add(ttl))
	} else {
		r.failures = 0
	}
	if ttl > 0 {
		r.storeLocked(key, githubSAMLCacheEntry{expiresAt: now.Add(ttl), login: login, err: err}, now)
	}
	call.login, call.err = login, err
	delete(r.inflight, key)
	close(call.done)
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// storeLocked bounds both positive and negative/transient state. Evict expired
// entries first, then the entry with the earliest expiry when at capacity.
func (r *GitHubSAMLResolver) storeLocked(key [sha256.Size]byte, entry githubSAMLCacheEntry, now time.Time) {
	var oldestKey [sha256.Size]byte
	var oldest time.Time
	for k, value := range r.cache {
		if !now.Before(value.expiresAt) {
			delete(r.cache, k)
			continue
		}
		if oldest.IsZero() || value.expiresAt.Before(oldest) {
			oldest, oldestKey = value.expiresAt, k
		}
	}
	if len(r.cache) >= githubSAMLMaxCacheEntries {
		delete(r.cache, oldestKey)
	}
	r.cache[key] = entry
}

// Keep Retry-After local and bounded; it is never exposed in API errors.
type githubSAMLStatusError struct {
	status     int
	retryAfter time.Duration
}

func (e *githubSAMLStatusError) Error() string {
	return fmt.Sprintf("GitHub GraphQL identity status %d", e.status)
}

func boundedRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	seconds, err := strconv.ParseInt(value, 10, 64)
	if errors.Is(err, strconv.ErrRange) && !strings.HasPrefix(value, "-") {
		return githubSAMLMaxBackoff
	}
	if err == nil {
		// Compare seconds before converting to Duration to avoid integer overflow.
		if seconds <= 0 {
			return 0
		}
		if seconds >= int64(githubSAMLMaxBackoff/time.Second) {
			return githubSAMLMaxBackoff
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return min(max(date.Sub(now), 0), githubSAMLMaxBackoff)
	}
	return 0
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
		return githubSAMLGraphQLResponse{}, &githubSAMLStatusError{status: resp.StatusCode, retryAfter: boundedRetryAfter(resp.Header.Get("Retry-After"), r.now())}
	}

	var graphQLResponse githubSAMLGraphQLResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, githubSAMLMaxResponseBytes)).Decode(&graphQLResponse); err != nil {
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
