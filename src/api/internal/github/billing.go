package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type BillingClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type AICreditUsageRequest struct {
	Enterprise   string
	User         string
	Organization string
	Model        string
	Product      string
	Year         int
	Month        int
	Day          int
}

type AICreditUsageReport struct {
	TimePeriod struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day,omitempty"`
	} `json:"timePeriod"`
	Enterprise string              `json:"enterprise"`
	UsageItems []AICreditUsageItem `json:"usageItems"`
}

type AICreditUsageItem struct {
	Product          string  `json:"product"`
	SKU              string  `json:"sku"`
	Model            string  `json:"model"`
	UnitType         string  `json:"unitType"`
	PricePerUnit     float64 `json:"pricePerUnit"`
	GrossQuantity    float64 `json:"grossQuantity"`
	GrossAmount      float64 `json:"grossAmount"`
	DiscountQuantity float64 `json:"discountQuantity"`
	DiscountAmount   float64 `json:"discountAmount"`
	NetQuantity      float64 `json:"netQuantity"`
	NetAmount        float64 `json:"netAmount"`
}

func NewBillingClient(baseURL, token string, httpClient *http.Client) *BillingClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BillingClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

func (c *BillingClient) GetAICreditUsage(ctx context.Context, usageRequest AICreditUsageRequest) (AICreditUsageReport, error) {
	endpoint := c.baseURL + "/enterprises/" + url.PathEscape(usageRequest.Enterprise) + "/settings/billing/ai_credit/usage"
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return AICreditUsageReport{}, fmt.Errorf("parse GitHub billing URL: %w", err)
	}

	query := requestURL.Query()
	addStringQuery(query, "month", strconv.Itoa(usageRequest.Month), usageRequest.Month > 0)
	addStringQuery(query, "user", usageRequest.User, usageRequest.User != "")
	addStringQuery(query, "year", strconv.Itoa(usageRequest.Year), usageRequest.Year > 0)
	addStringQuery(query, "day", strconv.Itoa(usageRequest.Day), usageRequest.Day > 0)
	addStringQuery(query, "organization", usageRequest.Organization, usageRequest.Organization != "")
	addStringQuery(query, "model", usageRequest.Model, usageRequest.Model != "")
	addStringQuery(query, "product", usageRequest.Product, usageRequest.Product != "")
	requestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return AICreditUsageReport{}, fmt.Errorf("create GitHub billing request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AICreditUsageReport{}, fmt.Errorf("request GitHub billing usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AICreditUsageReport{}, fmt.Errorf("GitHub billing usage status %d", resp.StatusCode)
	}

	var report AICreditUsageReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return AICreditUsageReport{}, fmt.Errorf("decode GitHub billing usage: %w", err)
	}
	return report, nil
}

func addStringQuery(query url.Values, key, value string, include bool) {
	if include {
		query.Set(key, value)
	}
}
