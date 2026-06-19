package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type FixtureBillingClient struct {
	path string
}

func NewFixtureBillingClient(path string) *FixtureBillingClient {
	return &FixtureBillingClient{path: path}
}

func (c *FixtureBillingClient) GetAICreditUsage(_ context.Context, usageRequest AICreditUsageRequest) (AICreditUsageReport, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return AICreditUsageReport{}, fmt.Errorf("read GitHub billing fixture: %w", err)
	}

	var report AICreditUsageReport
	if err := json.Unmarshal(data, &report); err != nil {
		return AICreditUsageReport{}, fmt.Errorf("decode GitHub billing fixture: %w", err)
	}
	report.TimePeriod.Year = usageRequest.Year
	report.TimePeriod.Month = usageRequest.Month
	report.TimePeriod.Day = usageRequest.Day
	report.Enterprise = usageRequest.Enterprise
	return report, nil
}
