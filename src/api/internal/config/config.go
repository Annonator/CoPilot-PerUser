package config

import (
	"fmt"
	"os"
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
	UsageCacheTTL               time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Port:                        envDefault("PORT", "8080"),
		CompanyEmailDomains:         splitCSV(os.Getenv("COMPANY_EMAIL_DOMAINS")),
		AppTokenSecret:              os.Getenv("APP_TOKEN_SECRET"),
		GitHubAPIBaseURL:            envDefault("GITHUB_API_BASE_URL", "https://api.github.com"),
		GitHubEnterpriseSlug:        os.Getenv("GITHUB_ENTERPRISE_SLUG"),
		GitHubAdminToken:            os.Getenv("GITHUB_ADMIN_TOKEN"),
		GitHubIdentityResolver:      envDefault("GITHUB_IDENTITY_RESOLVER", "static"),
		GitHubIdentityStaticMapPath: os.Getenv("GITHUB_IDENTITY_STATIC_MAP_PATH"),
		UsageCacheTTL:               10 * time.Minute,
	}

	if rawTTL := os.Getenv("USAGE_CACHE_TTL"); rawTTL != "" {
		ttl, err := time.ParseDuration(rawTTL)
		if err != nil {
			return Config{}, fmt.Errorf("parse USAGE_CACHE_TTL: %w", err)
		}
		cfg.UsageCacheTTL = ttl
	}

	if len(cfg.CompanyEmailDomains) == 0 {
		return Config{}, fmt.Errorf("COMPANY_EMAIL_DOMAINS is required")
	}
	if cfg.AppTokenSecret == "" {
		return Config{}, fmt.Errorf("APP_TOKEN_SECRET is required")
	}
	if cfg.GitHubEnterpriseSlug == "" {
		return Config{}, fmt.Errorf("GITHUB_ENTERPRISE_SLUG is required")
	}
	if cfg.GitHubAdminToken == "" {
		return Config{}, fmt.Errorf("GITHUB_ADMIN_TOKEN is required")
	}
	if cfg.GitHubIdentityResolver == "static" && cfg.GitHubIdentityStaticMapPath == "" {
		return Config{}, fmt.Errorf("GITHUB_IDENTITY_STATIC_MAP_PATH is required for static resolver")
	}

	return cfg, nil
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
