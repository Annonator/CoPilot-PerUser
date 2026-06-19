package config

import "testing"

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("COMPANY_EMAIL_DOMAINS", "company.name,example.com")
	t.Setenv("APP_TOKEN_SECRET", "secret")
	t.Setenv("GITHUB_API_BASE_URL", "https://api.github.com")
	t.Setenv("GITHUB_ENTERPRISE_SLUG", "marbis")
	t.Setenv("GITHUB_ADMIN_TOKEN", "ghp_secret")
	t.Setenv("GITHUB_IDENTITY_RESOLVER", "static")
	t.Setenv("GITHUB_IDENTITY_STATIC_MAP_PATH", "internal/testfixtures/identity-map.json")
	t.Setenv("USAGE_CACHE_TTL", "5m")

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
}

func TestLoadRequiresSecrets(t *testing.T) {
	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing required config")
	}
}
