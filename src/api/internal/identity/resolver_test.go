package identity

import (
	"context"
	"testing"
)

func TestStaticResolverResolve(t *testing.T) {
	resolver, err := NewStaticResolver("../testfixtures/identity-map.json")
	if err != nil {
		t.Fatalf("NewStaticResolver() error = %v", err)
	}
	login, err := resolver.ResolveGitHubLogin(context.Background(), "Andreas.Pohl@Nitrado.Net")
	if err != nil {
		t.Fatalf("ResolveGitHubLogin() error = %v", err)
	}
	if login != "Annonator" {
		t.Fatalf("login = %q", login)
	}
}

func TestStaticResolverNoMatch(t *testing.T) {
	resolver, err := NewStaticResolver("../testfixtures/identity-map.json")
	if err != nil {
		t.Fatalf("NewStaticResolver() error = %v", err)
	}
	_, err = resolver.ResolveGitHubLogin(context.Background(), "missing@company.name")
	if err == nil {
		t.Fatal("ResolveGitHubLogin() error = nil, want no match")
	}
}
