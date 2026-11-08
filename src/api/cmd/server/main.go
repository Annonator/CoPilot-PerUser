package main

import (
	"log"
	"net/http"
	"time"

	"copilot-per-user/api/internal/auth"
	"copilot-per-user/api/internal/budget"
	"copilot-per-user/api/internal/config"
	gh "copilot-per-user/api/internal/github"
	"copilot-per-user/api/internal/httpapi"
	"copilot-per-user/api/internal/identity"
	"copilot-per-user/api/internal/usage"
)

const (
	githubRequestTimeout  = 15 * time.Second
	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 60 * time.Second
	httpIdleTimeout       = 120 * time.Second
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	githubHTTPClient := newGitHubHTTPClient()
	var resolver identity.Resolver
	switch cfg.GitHubIdentityResolver {
	case "static":
		resolver, err = identity.NewStaticResolver(cfg.GitHubIdentityStaticMapPath)
		if err != nil {
			log.Fatalf("create static identity resolver: %v", err)
		}
	case "github_saml":
		resolver = identity.NewGitHubSAMLResolver(cfg.GitHubAPIBaseURL, cfg.GitHubAdminToken, cfg.GitHubEnterpriseSlug, cfg.UsageCacheTTL, githubHTTPClient)
	default:
		log.Fatalf("unsupported identity resolver: %s", cfg.GitHubIdentityResolver)
	}

	var billingClient usage.BillingClient
	var budgetClient budget.Client
	if cfg.GitHubBillingFixturePath != "" {
		fixtureClient := gh.NewFixtureBillingClient(cfg.GitHubBillingFixturePath)
		billingClient = fixtureClient
		budgetClient = fixtureClient
	} else {
		githubClient := gh.NewBillingClient(cfg.GitHubAPIBaseURL, cfg.GitHubAdminToken, githubHTTPClient)
		billingClient = githubClient
		budgetClient = githubClient
	}
	budgetResolver := budget.NewResolver(budget.ResolverConfig{
		Enterprise: cfg.GitHubEnterpriseSlug,
		Client:     budgetClient,
		CacheTTL:   cfg.UsageCacheTTL,
		Logf:       log.Printf,
	})
	usageService := usage.NewService(usage.ServiceConfig{
		Enterprise:            cfg.GitHubEnterpriseSlug,
		Resolver:              resolver,
		Billing:               billingClient,
		Budget:                budgetResolver,
		CacheTTL:              cfg.UsageCacheTTL,
		ReportingWindowMonths: cfg.UsageReportingWindowMonths,
	})
	server := httpapi.NewServer(httpapi.ServerConfig{
		Auth:                       auth.Manager{Secret: []byte(cfg.AppTokenSecret)},
		CompanyEmailDomains:        cfg.CompanyEmailDomains,
		Usage:                      usageService,
		UsageReportingWindowMonths: cfg.UsageReportingWindowMonths,
	})

	log.Printf("listening on :%s", cfg.Port)
	if err := newHTTPServer(":"+cfg.Port, server).ListenAndServe(); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}

// Client.Timeout covers dialing, redirects, headers, and response body reads.
func newGitHubHTTPClient() *http.Client {
	return &http.Client{
		Timeout: githubRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
