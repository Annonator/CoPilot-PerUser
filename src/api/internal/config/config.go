package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                        string
	CompanyEmailDomains         []string
	AppTokenSecret              string
	GitHubAPIBaseURL            string
	GitHubEnterpriseSlug        string
	GitHubAdminToken            string
	GitHubIdentityResolver      string
	GitHubIdentityStaticMapPath string
	GitHubBillingFixturePath    string
	UsageCacheTTL               time.Duration
	UsageReportingWindowMonths  int
}

const minimumSecretLength = 32

func Load() (Config, error) {
	cfg := Config{
		Port:                        envDefault("PORT", "8080"),
		CompanyEmailDomains:         splitCSV(os.Getenv("COMPANY_EMAIL_DOMAINS")),
		AppTokenSecret:              os.Getenv("APP_TOKEN_SECRET"),
		GitHubAPIBaseURL:            envDefault("GITHUB_API_BASE_URL", "https://api.github.com"),
		GitHubEnterpriseSlug:        os.Getenv("GITHUB_ENTERPRISE_SLUG"),
		GitHubAdminToken:            os.Getenv("GITHUB_ADMIN_TOKEN"),
		GitHubIdentityResolver:      strings.ToLower(strings.TrimSpace(envDefault("GITHUB_IDENTITY_RESOLVER", "static"))),
		GitHubIdentityStaticMapPath: os.Getenv("GITHUB_IDENTITY_STATIC_MAP_PATH"),
		GitHubBillingFixturePath:    os.Getenv("GITHUB_BILLING_FIXTURE_PATH"),
		UsageCacheTTL:               10 * time.Minute,
		UsageReportingWindowMonths:  6,
	}

	if rawTTL := os.Getenv("USAGE_CACHE_TTL"); rawTTL != "" {
		ttl, err := time.ParseDuration(rawTTL)
		if err != nil {
			return Config{}, fmt.Errorf("parse USAGE_CACHE_TTL: %w", err)
		}
		if ttl <= 0 {
			return Config{}, fmt.Errorf("USAGE_CACHE_TTL must be positive")
		}
		cfg.UsageCacheTTL = ttl
	}
	if rawWindow := os.Getenv("USAGE_REPORTING_WINDOW_MONTHS"); rawWindow != "" {
		window, err := strconv.Atoi(rawWindow)
		if err != nil {
			return Config{}, fmt.Errorf("parse USAGE_REPORTING_WINDOW_MONTHS: %w", err)
		}
		if window <= 0 {
			return Config{}, fmt.Errorf("USAGE_REPORTING_WINDOW_MONTHS must be positive")
		}
		cfg.UsageReportingWindowMonths = window
	}

	if len(cfg.CompanyEmailDomains) == 0 {
		return Config{}, fmt.Errorf("COMPANY_EMAIL_DOMAINS is required")
	}
	if err := validateAppTokenSecret(cfg.AppTokenSecret); err != nil {
		return Config{}, err
	}
	if cfg.GitHubEnterpriseSlug == "" {
		return Config{}, fmt.Errorf("GITHUB_ENTERPRISE_SLUG is required")
	}
	if cfg.GitHubAdminToken == "" && (cfg.GitHubBillingFixturePath == "" || cfg.GitHubIdentityResolver == "github_saml") {
		return Config{}, fmt.Errorf("GITHUB_ADMIN_TOKEN is required")
	}
	if cfg.GitHubBillingFixturePath != "" && os.Getenv("NODE_ENV") == "production" {
		return Config{}, fmt.Errorf("GITHUB_BILLING_FIXTURE_PATH is not allowed when NODE_ENV=production")
	}
	if cfg.GitHubIdentityResolver != "static" && cfg.GitHubIdentityResolver != "github_saml" {
		return Config{}, fmt.Errorf("GITHUB_IDENTITY_RESOLVER %q is unsupported; supported values are static and github_saml", cfg.GitHubIdentityResolver)
	}
	if cfg.GitHubIdentityResolver == "static" && cfg.GitHubIdentityStaticMapPath == "" {
		return Config{}, fmt.Errorf("GITHUB_IDENTITY_STATIC_MAP_PATH is required for static resolver")
	}

	return cfg, nil
}

func validateAppTokenSecret(secret string) error {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return fmt.Errorf("APP_TOKEN_SECRET is required")
	}
	if trimmed != secret {
		return fmt.Errorf("APP_TOKEN_SECRET must not include leading or trailing whitespace")
	}

	normalized := strings.ToLower(trimmed)
	if isPlaceholderSecret(normalized) || len(trimmed) < minimumSecretLength {
		return fmt.Errorf("APP_TOKEN_SECRET must be a long random value; placeholders and short secrets are not allowed")
	}

	return nil
}

func isPlaceholderSecret(secret string) bool {
	if strings.Contains(secret, "replace-with") || strings.Contains(secret, "placeholder") {
		return true
	}

	switch secret {
	case "changeme", "change-me", "secret", "test", "password",
		"local-app-token-secret", "local-auth-secret", "build-secret":
		return true
	default:
		return false
	}
}

func envDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
