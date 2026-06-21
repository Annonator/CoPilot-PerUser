package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const validAppTokenSecret = "0123456789abcdef0123456789abcdef"

func TestLoadFromEnv(t *testing.T) {
	setValidEnv(t)
	t.Setenv("USAGE_CACHE_TTL", "5m")
	t.Setenv("GITHUB_BILLING_FIXTURE_PATH", "internal/testfixtures/ai-credit-usage.json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "9090" {
		t.Fatalf("Port = %q", cfg.Port)
	}
	if got := cfg.CompanyEmailDomains[1]; got != "example.com" {
		t.Fatalf("CompanyEmailDomains[1] = %q", got)
	}
	if cfg.UsageCacheTTL.String() != "5m0s" {
		t.Fatalf("UsageCacheTTL = %s", cfg.UsageCacheTTL)
	}
	if cfg.GitHubBillingFixturePath != "internal/testfixtures/ai-credit-usage.json" {
		t.Fatalf("GitHubBillingFixturePath = %q", cfg.GitHubBillingFixturePath)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	for _, key := range []string{
		"COMPANY_EMAIL_DOMAINS",
		"APP_TOKEN_SECRET",
		"GITHUB_ENTERPRISE_SLUG",
		"GITHUB_ADMIN_TOKEN",
		"GITHUB_IDENTITY_STATIC_MAP_PATH",
	} {
		t.Run(key, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(key, "")

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want missing required config")
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("Load() error = %q, want it to contain %q", err.Error(), key)
			}
		})
	}
}

func TestLoadRejectsWeakAppTokenSecret(t *testing.T) {
	tests := []string{
		"replace-with-random-app-token-secret",
		"replace-with-another-long-random-secret",
		"local-app-token-secret",
		"secret",
		"short-secret",
	}

	for _, secret := range tests {
		t.Run(secret, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("APP_TOKEN_SECRET", secret)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want weak APP_TOKEN_SECRET rejection")
			}
			if !strings.Contains(err.Error(), "APP_TOKEN_SECRET") {
				t.Fatalf("Load() error = %q, want APP_TOKEN_SECRET context", err.Error())
			}
			if !regexp.MustCompile(`long random|placeholder`).MatchString(err.Error()) {
				t.Fatalf("Load() error = %q, want actionable secret guidance", err.Error())
			}
		})
	}
}

func TestLoadAcceptsLongAppTokenSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("APP_TOKEN_SECRET", validAppTokenSecret)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppTokenSecret != validAppTokenSecret {
		t.Fatalf("AppTokenSecret = %q", cfg.AppTokenSecret)
	}
}

func TestLoadDoesNotRequireGitHubAdminTokenForFixtureBilling(t *testing.T) {
	setValidEnv(t)
	t.Setenv("GITHUB_ADMIN_TOKEN", "")
	t.Setenv("GITHUB_BILLING_FIXTURE_PATH", "internal/testfixtures/ai-credit-usage.json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GitHubAdminToken != "" {
		t.Fatalf("GitHubAdminToken = %q, want empty", cfg.GitHubAdminToken)
	}
}

func TestLoadAcceptsGitHubSAMLIdentityResolver(t *testing.T) {
	setValidEnv(t)
	t.Setenv("GITHUB_IDENTITY_RESOLVER", "github_saml")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GitHubIdentityResolver != "github_saml" {
		t.Fatalf("GitHubIdentityResolver = %q", cfg.GitHubIdentityResolver)
	}
}

func TestLoadDoesNotRequireStaticMapForGitHubSAMLResolver(t *testing.T) {
	setValidEnv(t)
	t.Setenv("GITHUB_IDENTITY_RESOLVER", "github_saml")
	t.Setenv("GITHUB_IDENTITY_STATIC_MAP_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GitHubIdentityStaticMapPath != "" {
		t.Fatalf("GitHubIdentityStaticMapPath = %q, want empty", cfg.GitHubIdentityStaticMapPath)
	}
}

func TestLoadRequiresGitHubAdminTokenForGitHubSAMLResolver(t *testing.T) {
	setValidEnv(t)
	t.Setenv("GITHUB_IDENTITY_RESOLVER", "github_saml")
	t.Setenv("GITHUB_ADMIN_TOKEN", "")
	t.Setenv("GITHUB_BILLING_FIXTURE_PATH", "internal/testfixtures/ai-credit-usage.json")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing GITHUB_ADMIN_TOKEN")
	}
	if !strings.Contains(err.Error(), "GITHUB_ADMIN_TOKEN") {
		t.Fatalf("Load() error = %q, want GITHUB_ADMIN_TOKEN context", err.Error())
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	for _, key := range []string{
		"PORT",
		"GITHUB_API_BASE_URL",
		"GITHUB_IDENTITY_RESOLVER",
		"USAGE_CACHE_TTL",
	} {
		unsetEnv(t, key)
	}
	t.Setenv("COMPANY_EMAIL_DOMAINS", "company.name")
	t.Setenv("APP_TOKEN_SECRET", validAppTokenSecret)
	t.Setenv("GITHUB_ENTERPRISE_SLUG", "marbis")
	t.Setenv("GITHUB_ADMIN_TOKEN", "ghp_secret")
	t.Setenv("GITHUB_IDENTITY_STATIC_MAP_PATH", "internal/testfixtures/identity-map.json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port = %q", cfg.Port)
	}
	if cfg.GitHubAPIBaseURL != "https://api.github.com" {
		t.Fatalf("GitHubAPIBaseURL = %q", cfg.GitHubAPIBaseURL)
	}
	if cfg.GitHubIdentityResolver != "static" {
		t.Fatalf("GitHubIdentityResolver = %q", cfg.GitHubIdentityResolver)
	}
	if cfg.UsageCacheTTL.String() != "10m0s" {
		t.Fatalf("UsageCacheTTL = %s", cfg.UsageCacheTTL)
	}
}

func TestLoadRejectsNonPositiveUsageCacheTTL(t *testing.T) {
	for _, ttl := range []string{"0s", "-1m"} {
		t.Run(ttl, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("USAGE_CACHE_TTL", ttl)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want non-positive USAGE_CACHE_TTL error")
			}
		})
	}
}

func TestLoadRejectsUnsupportedIdentityResolver(t *testing.T) {
	setValidEnv(t)
	t.Setenv("GITHUB_IDENTITY_RESOLVER", "saml")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported resolver error")
	}
	if !strings.Contains(err.Error(), "GITHUB_IDENTITY_RESOLVER") {
		t.Fatalf("Load() error = %q, want GITHUB_IDENTITY_RESOLVER context", err.Error())
	}
	if !strings.Contains(err.Error(), "static") {
		t.Fatalf("Load() error = %q, want supported resolver context", err.Error())
	}
}

func TestLoadRejectsBillingFixtureInProduction(t *testing.T) {
	setValidEnv(t)
	t.Setenv("NODE_ENV", "production")
	t.Setenv("GITHUB_BILLING_FIXTURE_PATH", "internal/testfixtures/ai-credit-usage.json")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want production fixture rejection")
	}
	if !strings.Contains(err.Error(), "GITHUB_BILLING_FIXTURE_PATH") {
		t.Fatalf("Load() error = %q, want GITHUB_BILLING_FIXTURE_PATH context", err.Error())
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PORT", "9090")
	t.Setenv("COMPANY_EMAIL_DOMAINS", "company.name,example.com")
	t.Setenv("APP_TOKEN_SECRET", validAppTokenSecret)
	t.Setenv("GITHUB_API_BASE_URL", "https://api.github.com")
	t.Setenv("GITHUB_ENTERPRISE_SLUG", "marbis")
	t.Setenv("GITHUB_ADMIN_TOKEN", "ghp_secret")
	t.Setenv("GITHUB_IDENTITY_RESOLVER", "static")
	t.Setenv("GITHUB_IDENTITY_STATIC_MAP_PATH", "internal/testfixtures/identity-map.json")
	t.Setenv("USAGE_CACHE_TTL", "")
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}
