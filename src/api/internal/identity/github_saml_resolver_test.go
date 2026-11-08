package identity

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGitHubSAMLResolverResolvesLogin(t *testing.T) {
	var seenRequest graphQLRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/graphql" {
			t.Fatalf("path = %q, want /graphql", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !strings.Contains(seenRequest.Query, "externalIdentities") {
			t.Fatalf("query = %q, want externalIdentities", seenRequest.Query)
		}
		writeGraphQLResponse(t, w, `{
			"data": {
				"enterprise": {
					"ownerInfo": {
						"samlIdentityProvider": {
							"externalIdentities": {
								"nodes": [
									{
										"samlIdentity": {"nameId": "andreas.pohl@nitrado.net"},
										"user": {"login": "Annonator"}
									}
								]
							}
						}
					}
				}
			}
		}`)
	}))
	defer server.Close()

	resolver := NewGitHubSAMLResolver(server.URL, "ghp_secret", "marbis", time.Minute, server.Client())
	login, err := resolver.ResolveGitHubLogin(context.Background(), " Andreas.Pohl@Nitrado.Net ")
	if err != nil {
		t.Fatalf("ResolveGitHubLogin() error = %v", err)
	}
	if login != "Annonator" {
		t.Fatalf("login = %q", login)
	}
	if seenRequest.Variables["enterprise"] != "marbis" {
		t.Fatalf("enterprise variable = %#v", seenRequest.Variables["enterprise"])
	}
	if seenRequest.Variables["email"] != "andreas.pohl@nitrado.net" {
		t.Fatalf("email variable = %#v", seenRequest.Variables["email"])
	}
}

func TestGitHubSAMLResolverCachesSuccessfulLookup(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeGraphQLResponse(t, w, `{
			"data": {
				"enterprise": {
					"ownerInfo": {
						"samlIdentityProvider": {
							"externalIdentities": {
								"nodes": [
									{
										"samlIdentity": {"nameId": "andreas.pohl@nitrado.net"},
										"user": {"login": "Annonator"}
									}
								]
							}
						}
					}
				}
			}
		}`)
	}))
	defer server.Close()

	resolver := NewGitHubSAMLResolver(server.URL, "ghp_secret", "marbis", time.Minute, server.Client())
	for _, email := range []string{"andreas.pohl@nitrado.net", " Andreas.Pohl@Nitrado.Net "} {
		login, err := resolver.ResolveGitHubLogin(context.Background(), email)
		if err != nil {
			t.Fatalf("ResolveGitHubLogin(%q) error = %v", email, err)
		}
		if login != "Annonator" {
			t.Fatalf("login = %q", login)
		}
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestGitHubSAMLResolverReturnsNotFoundForNoMatch(t *testing.T) {
	resolver := newTestGitHubSAMLResolver(t, `{
		"data": {
			"enterprise": {
				"ownerInfo": {
					"samlIdentityProvider": {
						"externalIdentities": {"nodes": []}
					}
				}
			}
		}
	}`)

	_, err := resolver.ResolveGitHubLogin(context.Background(), "missing@nitrado.net")
	if !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("error = %v, want ErrIdentityNotFound", err)
	}
}

func TestGitHubSAMLResolverIncludesLinkedIdentitiesOutsideMembersOnlyFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if strings.Contains(request.Query, "membersOnly: true") {
			writeGraphQLResponse(t, w, `{
				"data": {
					"enterprise": {
						"ownerInfo": {
							"samlIdentityProvider": {
								"externalIdentities": {"nodes": []}
							}
						}
					}
				}
			}`)
			return
		}

		writeGraphQLResponse(t, w, `{
			"data": {
				"enterprise": {
					"ownerInfo": {
						"samlIdentityProvider": {
							"externalIdentities": {
								"nodes": [
									{
										"samlIdentity": {"nameId": "person@nitrado.net"},
										"user": {"login": "LinkedUser"}
									}
								]
							}
						}
					}
				}
			}
		}`)
	}))
	defer server.Close()

	resolver := NewGitHubSAMLResolver(server.URL, "ghp_secret", "marbis", time.Minute, server.Client())
	login, err := resolver.ResolveGitHubLogin(context.Background(), "person@nitrado.net")
	if err != nil {
		t.Fatalf("ResolveGitHubLogin() error = %v", err)
	}
	if login != "LinkedUser" {
		t.Fatalf("login = %q", login)
	}
}

func TestGitHubSAMLResolverReturnsNotFoundForMissingSAMLProvider(t *testing.T) {
	resolver := newTestGitHubSAMLResolver(t, `{
		"data": {
			"enterprise": {
				"ownerInfo": {
					"samlIdentityProvider": null
				}
			}
		}
	}`)

	_, err := resolver.ResolveGitHubLogin(context.Background(), "andreas.pohl@nitrado.net")
	if !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("error = %v, want ErrIdentityNotFound", err)
	}
}

func TestGitHubSAMLResolverReturnsNotFoundForUnclaimedIdentity(t *testing.T) {
	resolver := newTestGitHubSAMLResolver(t, `{
		"data": {
			"enterprise": {
				"ownerInfo": {
					"samlIdentityProvider": {
						"externalIdentities": {
							"nodes": [
								{
									"samlIdentity": {"nameId": "andreas.pohl@nitrado.net"},
									"user": null
								}
							]
						}
					}
				}
			}
		}
	}`)

	_, err := resolver.ResolveGitHubLogin(context.Background(), "andreas.pohl@nitrado.net")
	if !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("error = %v, want ErrIdentityNotFound", err)
	}
}

func TestGitHubSAMLResolverReturnsAmbiguousForDuplicateMatches(t *testing.T) {
	resolver := newTestGitHubSAMLResolver(t, `{
		"data": {
			"enterprise": {
				"ownerInfo": {
					"samlIdentityProvider": {
						"externalIdentities": {
							"nodes": [
								{
									"samlIdentity": {"nameId": "andreas.pohl@nitrado.net"},
									"user": {"login": "Annonator"}
								},
								{
									"samlIdentity": {"nameId": "andreas.pohl@nitrado.net"},
									"user": {"login": "OtherLogin"}
								}
							]
						}
					}
				}
			}
		}
	}`)

	_, err := resolver.ResolveGitHubLogin(context.Background(), "andreas.pohl@nitrado.net")
	if !errors.Is(err, ErrIdentityAmbiguous) {
		t.Fatalf("error = %v, want ErrIdentityAmbiguous", err)
	}
}

func TestGitHubSAMLResolverWrapsGraphQLErrors(t *testing.T) {
	resolver := newTestGitHubSAMLResolver(t, `{
		"errors": [
			{"message": "Resource protected by organization SAML enforcement"}
		]
	}`)

	_, err := resolver.ResolveGitHubLogin(context.Background(), "andreas.pohl@nitrado.net")
	if err == nil {
		t.Fatal("ResolveGitHubLogin() error = nil, want GraphQL error")
	}
	if !strings.Contains(err.Error(), "GitHub GraphQL identity error: Resource protected by organization SAML enforcement") {
		t.Fatalf("error = %q, want decoded GraphQL response error", err.Error())
	}
}

func TestGitHubSAMLResolverDoesNotLeakTokenInHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()

	resolver := NewGitHubSAMLResolver(server.URL, "ghp_super_secret", "marbis", time.Minute, server.Client())
	_, err := resolver.ResolveGitHubLogin(context.Background(), "andreas.pohl@nitrado.net")
	if err == nil {
		t.Fatal("ResolveGitHubLogin() error = nil, want status error")
	}
	if strings.Contains(err.Error(), "ghp_super_secret") {
		t.Fatalf("error leaked token: %q", err.Error())
	}
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func newTestGitHubSAMLResolver(t *testing.T, response string) *GitHubSAMLResolver {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeGraphQLResponse(t, w, response)
	}))
	t.Cleanup(server.Close)

	return NewGitHubSAMLResolver(server.URL, "ghp_secret", "marbis", time.Minute, server.Client())
}

func writeGraphQLResponse(t *testing.T, w http.ResponseWriter, response string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(response)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func TestGitHubSAMLResolverCachesMissingIdentity(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeGraphQLResponse(t, w, `{"data":{"enterprise":{"ownerInfo":{"samlIdentityProvider":{"externalIdentities":{"nodes":[]}}}}}}`)
	}))
	defer server.Close()
	resolver := NewGitHubSAMLResolver(server.URL, "secret", "company", time.Minute, server.Client())
	for _, email := range []string{"missing@company.test", " Missing@Company.Test "} {
		if _, err := resolver.ResolveGitHubLogin(context.Background(), email); !errors.Is(err, ErrIdentityNotFound) {
			t.Fatalf("error = %v, want ErrIdentityNotFound", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

// The clock advances without sleeps; upstream work and cancellation still use
// real contexts so these tests exercise the actual shared operation boundary.
type samlTestClock struct{ nanos atomic.Int64 }

func newSAMLTestClock() *samlTestClock {
	c := &samlTestClock{}
	c.nanos.Store(time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC).UnixNano())
	return c
}
func (c *samlTestClock) now() time.Time          { return time.Unix(0, c.nanos.Load()) }
func (c *samlTestClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

type samlRoundTripper func(*http.Request) (*http.Response, error)

func (f samlRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func samlResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func samlNodes(nodes string) string {
	return `{"data":{"enterprise":{"ownerInfo":{"samlIdentityProvider":{"externalIdentities":{"nodes":[` + nodes + `]}}}}}}`
}
func samlLinked(email, login string) string {
	return samlNodes(fmt.Sprintf(`{"samlIdentity":{"nameId":%q},"user":{"login":%q}}`, email, login))
}

func TestGitHubSAMLResolverSharesConcurrentOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantErr    error
	}{
		{"success", samlLinked("person@company.test", "Person"), 200, nil},
		{"missing", samlNodes(""), 200, ErrIdentityNotFound},
		{"ambiguous", samlNodes(`{"samlIdentity":{"nameId":"person@company.test"},"user":{"login":"one"}},{"samlIdentity":{"nameId":"person@company.test"},"user":{"login":"two"}}`), 200, ErrIdentityAmbiguous},
		{"upstream", "unavailable", 503, errGitHubSAMLBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
				if requests.Add(1) == 1 {
					close(entered)
				}
				<-release
				return samlResponse(tc.status, tc.body), nil
			})}
			resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
			var wg sync.WaitGroup
			results := make(chan error, 40)
			for i := 0; i < 40; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					email := "person@company.test"
					if i%2 == 0 {
						email = " Person@Company.Test "
					}
					login, err := resolver.ResolveGitHubLogin(context.Background(), email)
					if tc.wantErr == nil && (err != nil || login != "Person") {
						results <- fmt.Errorf("login=%q err=%v", login, err)
					} else if tc.wantErr == errGitHubSAMLBusy && err == nil {
						results <- errors.New("missing upstream error")
					} else if tc.wantErr != nil && tc.wantErr != errGitHubSAMLBusy && !errors.Is(err, tc.wantErr) {
						results <- fmt.Errorf("error=%v, want %v", err, tc.wantErr)
					}
				}(i)
			}
			<-entered
			close(release)
			wg.Wait()
			close(results)
			for err := range results {
				t.Error(err)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("upstream requests=%d, want 1", got)
			}
		})
	}
}

func TestGitHubSAMLResolverRefreshesEachCachedState(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ttl        time.Duration
		wantErr    error
	}{
		{"positive", samlLinked("person@company.test", "Before"), 200, time.Minute, nil},
		{"missing", samlNodes(""), 200, githubSAMLNegativeTTL, ErrIdentityNotFound},
		{"ambiguous", samlNodes(`{"samlIdentity":{"nameId":"person@company.test"},"user":{"login":"one"}},{"samlIdentity":{"nameId":"person@company.test"},"user":{"login":"two"}}`), 200, githubSAMLNegativeTTL, ErrIdentityAmbiguous},
		{"HTTP", "failure", 503, githubSAMLBackoff, errGitHubSAMLBusy},
		{"GraphQL", `{"errors":[{"message":"denied"}]}`, 200, githubSAMLBackoff, errGitHubSAMLBusy},
		{"malformed", `{`, 200, githubSAMLBackoff, errGitHubSAMLBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := newSAMLTestClock()
			var requests atomic.Int32
			client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
				if requests.Add(1) == 1 {
					return samlResponse(tc.status, tc.body), nil
				}
				return samlResponse(200, samlLinked("person@company.test", "After")), nil
			})}
			resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
			resolver.now = clock.now
			_, originalErr := resolver.ResolveGitHubLogin(context.Background(), "person@company.test")
			if tc.wantErr == nil && originalErr != nil {
				t.Fatal(originalErr)
			}
			if tc.wantErr != nil && originalErr == nil {
				t.Fatal("expected original failure")
			}
			if tc.wantErr != nil && tc.wantErr != errGitHubSAMLBusy && !errors.Is(originalErr, tc.wantErr) {
				t.Fatalf("error=%v, want %v", originalErr, tc.wantErr)
			}
			clock.advance(tc.ttl - time.Nanosecond)
			if _, err := resolver.ResolveGitHubLogin(context.Background(), " PERSON@COMPANY.TEST "); err != originalErr {
				t.Fatalf("cached error=%v, want %v", err, originalErr)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("before expiry requests=%d", got)
			}
			clock.advance(time.Nanosecond)
			login, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test")
			if err != nil || login != "After" {
				t.Fatalf("after expiry login=%q err=%v", login, err)
			}
			if got := requests.Load(); got != 2 {
				t.Fatalf("after expiry requests=%d", got)
			}
		})
	}
}

func TestGitHubSAMLResolverBoundsDistinctIdentities(t *testing.T) {
	clock := newSAMLTestClock()
	var requests, active, peak atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, githubSAMLMaxConcurrent)
	client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
		requests.Add(1)
		n := active.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return samlResponse(200, samlNodes("")), nil
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
	resolver.now = clock.now
	var wg sync.WaitGroup
	for i := 0; i < githubSAMLMaxConcurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = resolver.ResolveGitHubLogin(context.Background(), fmt.Sprintf("person%d@company.test", i))
		}(i)
		<-started
		// A distinct identity cannot start inside the minimum request interval.
		if _, err := resolver.ResolveGitHubLogin(context.Background(), "rate-bound@company.test"); !errors.Is(err, errGitHubSAMLBusy) {
			t.Fatalf("rate admission error=%v", err)
		}
		clock.advance(githubSAMLRequestInterval)
	}
	if _, err := resolver.ResolveGitHubLogin(context.Background(), "concurrency-bound@company.test"); !errors.Is(err, errGitHubSAMLBusy) {
		t.Fatalf("concurrency admission error=%v", err)
	}
	close(release)
	wg.Wait()
	if got := requests.Load(); got != githubSAMLMaxConcurrent {
		t.Fatalf("requests=%d", got)
	}
	if got := peak.Load(); got != githubSAMLMaxConcurrent {
		t.Fatalf("peak concurrency=%d", got)
	}
}

func TestGitHubSAMLResolverTransientBackoffAcrossIdentities(t *testing.T) {
	clock := newSAMLTestClock()
	var requests atomic.Int32
	client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("network unavailable")
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
	resolver.now = clock.now
	for i, delay := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, time.Minute, time.Minute} {
		_, err := resolver.ResolveGitHubLogin(context.Background(), fmt.Sprintf("person%d@company.test", i))
		if err == nil || errors.Is(err, errGitHubSAMLBusy) {
			t.Fatalf("admitted failure=%v", err)
		}
		clock.advance(delay - time.Nanosecond)
		if _, err := resolver.ResolveGitHubLogin(context.Background(), "other@company.test"); !errors.Is(err, errGitHubSAMLBusy) {
			t.Fatalf("cooldown error=%v", err)
		}
		if got := requests.Load(); got != int32(i+1) {
			t.Fatalf("requests=%d", got)
		}
		clock.advance(time.Nanosecond)
	}
}

func TestGitHubSAMLResolverHonorsBoundedRetryAfter(t *testing.T) {
	for _, value := range []string{"40", "999999999999999999", "999999999999999999999999999999", "Sun, 08 Nov 2026 12:00:40 GMT"} {
		t.Run(value, func(t *testing.T) {
			clock := newSAMLTestClock()
			var requests atomic.Int32
			client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
				if requests.Add(1) == 1 {
					response := samlResponse(429, "rate limit")
					response.Header.Set("Retry-After", value)
					return response, nil
				}
				return samlResponse(200, samlLinked("person@company.test", "Recovered")), nil
			})}
			resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
			resolver.now = clock.now
			if _, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test"); err == nil {
				t.Fatal("expected 429 error")
			}
			delay := 40 * time.Second
			if strings.HasPrefix(value, "999") {
				delay = githubSAMLMaxBackoff
			}
			clock.advance(delay - time.Nanosecond)
			if _, err := resolver.ResolveGitHubLogin(context.Background(), "other@company.test"); !errors.Is(err, errGitHubSAMLBusy) {
				t.Fatalf("before Retry-After error=%v", err)
			}
			clock.advance(time.Nanosecond)
			login, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test")
			if err != nil || login != "Recovered" {
				t.Fatalf("recovered login=%q err=%v", login, err)
			}
			if got := requests.Load(); got != 2 {
				t.Fatalf("requests=%d", got)
			}
		})
	}
	now := newSAMLTestClock().now()
	for _, value := range []string{"-1", "invalid", "Sun, 08 Nov 2026 11:00:00 GMT"} {
		if got := boundedRetryAfter(value, now); got != 0 {
			t.Fatalf("Retry-After(%q)=%v", value, got)
		}
	}
}

func TestGitHubSAMLResolverCallerCancellationDoesNotPoisonSharedLookup(t *testing.T) {
	var requests atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	client := &http.Client{Transport: samlRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		if _, ok := req.Context().Deadline(); !ok {
			t.Error("upstream lookup has no deadline")
		}
		close(entered)
		select {
		case <-release:
			return samlResponse(200, samlLinked("person@company.test", "Person")), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	ownerResult := make(chan error, 1)
	go func() { _, err := resolver.ResolveGitHubLogin(ownerCtx, "person@company.test"); ownerResult <- err }()
	<-entered
	cancelOwner()
	select {
	case err := <-ownerResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("owner did not stop waiting")
	}
	waiterCtx, cancelWaiter := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelWaiter()
	if _, err := resolver.ResolveGitHubLogin(waiterCtx, " PERSON@COMPANY.TEST "); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter error=%v", err)
	}
	liveResult := make(chan error, 1)
	go func() {
		login, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test")
		if err == nil && login != "Person" {
			err = fmt.Errorf("login=%q", login)
		}
		liveResult <- err
	}()
	close(release)
	if err := <-liveResult; err != nil {
		t.Fatal(err)
	}
	if login, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test"); err != nil || login != "Person" {
		t.Fatalf("cached login=%q err=%v", login, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests=%d, want1", got)
	}
}

func TestGitHubSAMLResolverBoundsUpstreamDuration(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: samlRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
	resolver.operationTimeout = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := resolver.ResolveGitHubLogin(ctx, "person@company.test"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("upstream timeout error=%v", err)
	}
	if _, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cached timeout error=%v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("timeout requests=%d", got)
	}
}

func TestGitHubSAMLResolverZeroPositiveTTLStillProtectsFailures(t *testing.T) {
	clock := newSAMLTestClock()
	var requests atomic.Int32
	client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
		requests.Add(1)
		return samlResponse(200, samlNodes("")), nil
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", 0, client)
	resolver.now = clock.now
	for i := 0; i < 2; i++ {
		if _, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test"); !errors.Is(err, ErrIdentityNotFound) {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("negative requests=%d", got)
	}
}

func TestGitHubSAMLResolverBoundsCacheMemory(t *testing.T) {
	clock := newSAMLTestClock()
	var requests atomic.Int32
	client := &http.Client{Transport: samlRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		var body githubSAMLGraphQLRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		return samlResponse(200, samlLinked(body.Variables["email"], "Person")), nil
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", 24*time.Hour, client)
	resolver.now = clock.now
	for i := 0; i <= githubSAMLMaxCacheEntries; i++ {
		if _, err := resolver.ResolveGitHubLogin(context.Background(), fmt.Sprintf("person%d@company.test", i)); err != nil {
			t.Fatal(err)
		}
		clock.advance(githubSAMLRequestInterval)
	}
	if got := len(resolver.cache); got != githubSAMLMaxCacheEntries {
		t.Fatalf("cache entries=%d", got)
	}
	oldest := sha256.Sum256([]byte("person0@company.test"))
	if _, ok := resolver.cache[oldest]; ok {
		t.Fatal("oldest expiry was not evicted")
	}
	// Recently cached users remain usable even while fresh lookups are throttled.
	resolver.mu.Lock()
	resolver.nextRequestAt = clock.now().Add(time.Minute)
	resolver.mu.Unlock()
	if _, err := resolver.ResolveGitHubLogin(context.Background(), fmt.Sprintf("person%d@company.test", githubSAMLMaxCacheEntries)); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != githubSAMLMaxCacheEntries+1 {
		t.Fatalf("requests=%d", got)
	}
	clock.advance(24 * time.Hour)
	if _, err := resolver.ResolveGitHubLogin(context.Background(), "fresh@company.test"); err != nil {
		t.Fatal(err)
	}
	if got := len(resolver.cache); got != 1 {
		t.Fatalf("expired cache entries retained=%d", got)
	}
}

func TestGitHubSAMLResolverRejectsAlreadyCanceledCallerWithoutUpstream(t *testing.T) {
	client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
		t.Error("canceled caller started upstream request")
		return samlResponse(200, samlNodes("")), nil
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveGitHubLogin(ctx, "person@company.test"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestGitHubSAMLResolverBoundsRetainedResponse(t *testing.T) {
	client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
		return samlResponse(200, samlLinked("person@company.test", strings.Repeat("x", githubSAMLMaxResponseBytes))), nil
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", time.Minute, client)
	if _, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test"); err == nil || errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("oversized response error=%v", err)
	}
}

func TestGitHubSAMLResolverZeroPositiveTTLRefreshesSuccessfulIdentity(t *testing.T) {
	clock := newSAMLTestClock()
	var requests atomic.Int32
	client := &http.Client{Transport: samlRoundTripper(func(_ *http.Request) (*http.Response, error) {
		n := requests.Add(1)
		return samlResponse(200, samlLinked("person@company.test", fmt.Sprintf("Person%d", n))), nil
	})}
	resolver := NewGitHubSAMLResolver("https://github.test", "secret", "company", 0, client)
	resolver.now = clock.now
	for i := 1; i <= 2; i++ {
		login, err := resolver.ResolveGitHubLogin(context.Background(), "person@company.test")
		if err != nil || login != fmt.Sprintf("Person%d", i) {
			t.Fatalf("login=%q err=%v", login, err)
		}
		clock.advance(githubSAMLRequestInterval)
	}
	if len(resolver.cache) != 0 {
		t.Fatal("zero positive TTL retained success")
	}
}
