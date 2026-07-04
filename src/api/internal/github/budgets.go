package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type BudgetListRequest struct {
	Enterprise string
	Scope      string
	User       string
	Page       int
	PerPage    int
}

type GetBudgetRequest struct {
	Enterprise string
	BudgetID   string
}

type BudgetUserStatesRequest struct {
	Enterprise          string
	BudgetID            string
	User                string
	Page                int
	PerPage             int
	SortOrder           string
	ThresholdLowerBound int
	ThresholdUpperBound int
}

type BudgetListResponse struct {
	Budgets     []Budget `json:"budgets"`
	HasNextPage bool     `json:"has_next_page"`
	TotalCount  int      `json:"total_count"`
}

type Budget struct {
	ID                  string   `json:"id"`
	BudgetType          string   `json:"budget_type"`
	BudgetProductSKU    string   `json:"budget_product_sku"`
	BudgetProductSKUs   []string `json:"budget_product_skus"`
	BudgetScope         string   `json:"budget_scope"`
	BudgetEntityName    string   `json:"budget_entity_name"`
	BudgetAmount        float64  `json:"budget_amount"`
	PreventFurtherUsage bool     `json:"prevent_further_usage"`
}

type BudgetUserStatesResponse struct {
	UserStates  []BudgetUserState `json:"user_states"`
	HasNextPage bool              `json:"has_next_page"`
	TotalCount  int               `json:"total_count"`
}

type BudgetUserState struct {
	User             string  `json:"user"`
	ConsumedAmount   float64 `json:"consumed_amount"`
	TargetAmount     float64 `json:"target_amount"`
	OverrideBudgetID string  `json:"override_budget_id"`
}

func (c *BillingClient) ListBudgets(ctx context.Context, req BudgetListRequest) (BudgetListResponse, error) {
	endpoint := c.baseURL + "/enterprises/" + url.PathEscape(req.Enterprise) + "/settings/billing/budgets"
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return BudgetListResponse{}, fmt.Errorf("parse GitHub budgets URL: %w", err)
	}

	query := requestURL.Query()
	addStringQuery(query, "scope", req.Scope, req.Scope != "")
	addStringQuery(query, "user", req.User, req.User != "")
	addStringQuery(query, "page", strconv.Itoa(req.Page), req.Page > 0)
	addStringQuery(query, "per_page", strconv.Itoa(req.PerPage), req.PerPage > 0)
	requestURL.RawQuery = query.Encode()

	var response BudgetListResponse
	if err := c.getJSON(ctx, requestURL.String(), "GitHub budgets", &response); err != nil {
		return BudgetListResponse{}, err
	}
	return response, nil
}

func (c *BillingClient) GetBudget(ctx context.Context, req GetBudgetRequest) (Budget, error) {
	endpoint := c.baseURL + "/enterprises/" + url.PathEscape(req.Enterprise) + "/settings/billing/budgets/" + url.PathEscape(req.BudgetID)
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return Budget{}, fmt.Errorf("parse GitHub budget URL: %w", err)
	}

	var budget Budget
	if err := c.getJSON(ctx, requestURL.String(), "GitHub budget", &budget); err != nil {
		return Budget{}, err
	}
	return budget, nil
}

func (c *BillingClient) GetBudgetUserStates(ctx context.Context, req BudgetUserStatesRequest) (BudgetUserStatesResponse, error) {
	endpoint := c.baseURL + "/enterprises/" + url.PathEscape(req.Enterprise) + "/settings/billing/budgets/" + url.PathEscape(req.BudgetID) + "/user-states"
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return BudgetUserStatesResponse{}, fmt.Errorf("parse GitHub budget user states URL: %w", err)
	}

	query := requestURL.Query()
	addStringQuery(query, "user", req.User, req.User != "")
	addStringQuery(query, "page", strconv.Itoa(req.Page), req.Page > 0)
	addStringQuery(query, "per_page", strconv.Itoa(req.PerPage), req.PerPage > 0)
	addStringQuery(query, "sort_order", req.SortOrder, req.SortOrder != "")
	addStringQuery(query, "threshold_lower_bound", strconv.Itoa(req.ThresholdLowerBound), req.ThresholdLowerBound > 0)
	addStringQuery(query, "threshold_upper_bound", strconv.Itoa(req.ThresholdUpperBound), req.ThresholdUpperBound > 0)
	requestURL.RawQuery = query.Encode()

	var response BudgetUserStatesResponse
	if err := c.getJSON(ctx, requestURL.String(), "GitHub budget user states", &response); err != nil {
		return BudgetUserStatesResponse{}, err
	}
	return response, nil
}

func (c *BillingClient) getJSON(ctx context.Context, requestURL string, label string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create %s request: %w", label, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s status %d", label, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	return nil
}
