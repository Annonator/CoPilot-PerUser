package github

import (
	"context"
	"testing"
)

func TestFixtureBillingClientReturnsFixtureReport(t *testing.T) {
	client := NewFixtureBillingClient("../testfixtures/ai-credit-usage.json")

	report, err := client.GetAICreditUsage(context.Background(), AICreditUsageRequest{
		Enterprise: "marbis",
		User:       "Annonator",
		Year:       2026,
		Month:      6,
		Day:        2,
	})
	if err != nil {
		t.Fatalf("GetAICreditUsage() error = %v", err)
	}
	if report.Enterprise != "marbis" {
		t.Fatalf("Enterprise = %q", report.Enterprise)
	}
	if report.TimePeriod.Year != 2026 || report.TimePeriod.Month != 6 || report.TimePeriod.Day != 2 {
		t.Fatalf("TimePeriod = %#v", report.TimePeriod)
	}
	if len(report.UsageItems) != 2 {
		t.Fatalf("UsageItems length = %d", len(report.UsageItems))
	}
}
