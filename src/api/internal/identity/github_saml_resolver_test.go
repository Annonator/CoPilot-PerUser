package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if !strings.Contains(err.Error(), "GitHub GraphQL") {
		t.Fatalf("error = %q, want GitHub GraphQL context", err.Error())
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
