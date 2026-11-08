package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gh "copilot-per-user/api/internal/github"
)

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	handler := http.NewServeMux()

	server := newHTTPServer(":9090", handler)

	if server.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", server.Addr)
	}
	if server.Handler != handler {
		t.Fatal("Handler was not preserved")
	}

	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "ReadHeaderTimeout", got: server.ReadHeaderTimeout, want: 5 * time.Second},
		{name: "ReadTimeout", got: server.ReadTimeout, want: 15 * time.Second},
		{name: "WriteTimeout", got: server.WriteTimeout, want: 60 * time.Second},
		{name: "IdleTimeout", got: server.IdleTimeout, want: 120 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %s, want %s", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestGitHubHTTPClientBoundsHeadersAndResponseBody(t *testing.T) {
	for _, stalledBody := range []bool{false, true} {
		name := "headers"
		if stalledBody {
			name = "body"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stalledBody {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("{"))
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			client := newGitHubHTTPClient()
			if client == http.DefaultClient || client.Timeout != 15*time.Second {
				t.Fatalf("client = %#v, want dedicated client with 15s timeout", client)
			}
			client.Timeout = 30 * time.Millisecond
			billing := gh.NewBillingClient(server.URL, "test-token", client)
			_, err := billing.GetAICreditUsage(context.Background(), gh.AICreditUsageRequest{Enterprise: "test", User: "alice", Year: 2026, Month: 6})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("stalled GitHub request error = %v", err)
			}
		})
	}
}

func TestGitHubHTTPClientDoesNotFollowRedirects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	client := newGitHubHTTPClient()
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound || requests != 1 {
		t.Fatalf("status = %d requests = %d, want one 302 response", resp.StatusCode, requests)
	}
}
