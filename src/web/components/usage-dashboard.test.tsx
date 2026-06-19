import "@testing-library/jest-dom/vitest";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { UsageDashboard } from "./usage-dashboard";
import type { MonthlyUsage } from "@/lib/usage-types";

const usage = {
  period: {
    year: 2026,
    month: 6
  },
  user: {
    email: "ana@company.name",
    githubLogin: "ana"
  },
  totals: {
    includedCredits: 1250,
    additionalCredits: 320,
    grossAmount: 15.7,
    additionalUsage: 3.2
  },
  daily: [
    {
      day: "2026-06-01",
      totals: {
        includedCredits: 900,
        additionalCredits: 100,
        grossAmount: 10,
        additionalUsage: 1
      },
      models: [
        {
          model: "gpt-4.1",
          includedCredits: 500,
          additionalCredits: 40,
          grossAmount: 5.4,
          additionalUsage: 0.4,
          pricePerCredit: 0.01
        }
      ]
    },
    {
      day: "2026-06-02",
      totals: {
        includedCredits: 350,
        additionalCredits: 220,
        grossAmount: 5.7,
        additionalUsage: 2.2
      },
      models: [
        {
          model: "claude-3.7-sonnet",
          includedCredits: 300,
          additionalCredits: 180,
          grossAmount: 4.8,
          additionalUsage: 1.8,
          pricePerCredit: 0.01
        }
      ]
    }
  ],
  models: [
    {
      model: "gpt-4.1",
      includedCredits: 700,
      additionalCredits: 80,
      grossAmount: 7.8,
      additionalUsage: 0.8,
      pricePerCredit: 0.01
    },
    {
      model: "claude-3.7-sonnet",
      includedCredits: 550,
      additionalCredits: 240,
      grossAmount: 7.9,
      additionalUsage: 2.4,
      pricePerCredit: 0.01
    }
  ],
  sourceMetadata: {
    enterprise: "marbis",
    source: "github_enterprise_billing_ai_credit_usage",
    cached: true
  }
} satisfies MonthlyUsage;

describe("UsageDashboard", () => {
  it("renders usage totals, daily rows, and model breakdown rows", () => {
    render(<UsageDashboard usage={usage} />);

    expect(screen.getByRole("heading", { name: "2026-06" })).toBeInTheDocument();
    expect(screen.getByText("@ana")).toBeInTheDocument();
    expect(screen.getByText("ana@company.name")).toBeInTheDocument();
    expect(screen.getByText("1,250")).toBeInTheDocument();
    expect(screen.getByText("320")).toBeInTheDocument();
    expect(screen.getByText("$15.70")).toBeInTheDocument();
    expect(screen.getByText("$3.20")).toBeInTheDocument();
    expect(screen.queryByText("Price per credit $0.00")).not.toBeInTheDocument();

    const daily = screen.getByRole("region", { name: /daily usage/i });
    expect(within(daily).getByText("Jun 1")).toBeInTheDocument();
    expect(within(daily).getByText("1,000 credits")).toBeInTheDocument();
    expect(within(daily).getByText("$1.00 additional usage")).toBeInTheDocument();
    expect(within(daily).getByText("Jun 2")).toBeInTheDocument();
    expect(within(daily).getByText("570 credits")).toBeInTheDocument();
    expect(within(daily).getByText("$2.20 additional usage")).toBeInTheDocument();

    const table = screen.getByRole("table", { name: /model breakdown/i });
    expect(within(table).getByRole("columnheader", { name: /price per credit/i })).toBeInTheDocument();
    expect(within(table).getByRole("row", { name: /gpt-4.1 700 80 \$7.80 \$0.80 \$0.01/i })).toBeInTheDocument();
    expect(within(table).getByRole("row", { name: /claude-3.7-sonnet 550 240 \$7.90 \$2.40 \$0.01/i })).toBeInTheDocument();
  });

  it("renders partial empty states for missing daily or model usage", () => {
    render(<UsageDashboard usage={{ ...usage, daily: [], models: [] }} />);

    const daily = screen.getByRole("region", { name: /daily usage/i });
    expect(within(daily).getByText("No daily usage")).toBeInTheDocument();

    const table = screen.getByRole("table", { name: /model breakdown/i });
    expect(within(table).getByText("No model usage")).toBeInTheDocument();
  });
});
