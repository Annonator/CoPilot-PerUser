package main

import (
	"log"
	"net/http"

	"copilot-per-user/api/internal/auth"
	"copilot-per-user/api/internal/config"
	gh "copilot-per-user/api/internal/github"
	"copilot-per-user/api/internal/httpapi"
	"copilot-per-user/api/internal/identity"
	"copilot-per-user/api/internal/usage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	var resolver identity.Resolver
	switch cfg.GitHubIdentityResolver {
	case "static":
		resolver, err = identity.NewStaticResolver(cfg.GitHubIdentityStaticMapPath)
		if err != nil {
			log.Fatalf("create static identity resolver: %v", err)
		}
	case "github_saml":
		resolver = identity.NewGitHubSAMLResolver(cfg.GitHubAPIBaseURL, cfg.GitHubAdminToken, cfg.GitHubEnterpriseSlug, cfg.UsageCacheTTL, http.DefaultClient)
	default:
		log.Fatalf("unsupported identity resolver: %s", cfg.GitHubIdentityResolver)
	}

	var billingClient usage.BillingClient
	if cfg.GitHubBillingFixturePath != "" {
		billingClient = gh.NewFixtureBillingClient(cfg.GitHubBillingFixturePath)
	} else {
		billingClient = gh.NewBillingClient(cfg.GitHubAPIBaseURL, cfg.GitHubAdminToken, http.DefaultClient)
	}
	usageService := usage.NewService(usage.ServiceConfig{
		Enterprise: cfg.GitHubEnterpriseSlug,
		Resolver:   resolver,
		Billing:    billingClient,
		CacheTTL:   cfg.UsageCacheTTL,
	})
	server := httpapi.NewServer(httpapi.ServerConfig{
		Auth:                auth.Manager{Secret: []byte(cfg.AppTokenSecret)},
		CompanyEmailDomains: cfg.CompanyEmailDomains,
		Usage:               usageService,
	})

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, server); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}
