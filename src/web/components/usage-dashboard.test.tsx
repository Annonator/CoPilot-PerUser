import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { UsageDashboard } from "./usage-dashboard";
import type { MonthlyUsage, UserBudget } from "@/lib/usage-types";

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

function usageWithBudget(budget: UserBudget): MonthlyUsage {
  return {
    ...usage,
    budget
  };
}

function usageWithDayCount(dayCount: number): MonthlyUsage {
  return {
    ...usage,
    daily: Array.from({ length: dayCount }, (_, index) => {
      const dayNumber = index + 1;
      return {
        day: `2026-06-${String(dayNumber).padStart(2, "0")}`,
        totals: {
          includedCredits: 20 + dayNumber,
          additionalCredits: dayNumber,
          grossAmount: dayNumber,
          additionalUsage: dayNumber / 10
        },
        models: []
      };
    })
  };
}

describe("UsageDashboard", () => {
  beforeEach(() => {
    const storage = new Map<string, string>();
    const localStorageMock = {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => {
        storage.set(key, value);
      }),
      removeItem: vi.fn((key: string) => {
        storage.delete(key);
      }),
      clear: vi.fn(() => {
        storage.clear();
      })
    } satisfies Pick<Storage, "getItem" | "setItem" | "removeItem" | "clear">;

    vi.stubGlobal("localStorage", localStorageMock);
    document.documentElement.removeAttribute("data-theme");
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders the compact dashboard shell with summary rail and model-first workspace", () => {
    render(<UsageDashboard usage={usage} />);

    expect(screen.getByRole("main", { name: /copilot usage dashboard/i })).toBeInTheDocument();
    expect(screen.getByRole("complementary", { name: /usage summary/i })).toBeInTheDocument();

    const modelHeading = screen.getByRole("heading", { name: "Usage by model" });
    const dailyHeading = screen.getByRole("heading", { name: "Daily usage" });
    expect(modelHeading).toBeInTheDocument();
    expect(modelHeading.compareDocumentPosition(dailyHeading)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);

    expect(screen.getAllByText("Jun 2026")).toHaveLength(1);
    expect(screen.queryByText("Period")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "@ana" })).toHaveAttribute("href", "https://github.com/ana");
    expect(screen.getByText("ana@company.name")).toBeInTheDocument();

    const summary = screen.getByRole("complementary", { name: /usage summary/i });
    expect(within(summary).getByText("Included credits")).toBeInTheDocument();
    expect(within(summary).getByText("Additional credits")).toBeInTheDocument();
    expect(within(summary).getByText("Gross amount")).toBeInTheDocument();
    expect(within(summary).getByText("Additional usage")).toBeInTheDocument();
    expect(within(summary).getByText("1,250")).toBeInTheDocument();
    expect(within(summary).getByText("320")).toBeInTheDocument();
    expect(within(summary).getByText("$15.70")).toBeInTheDocument();
    expect(within(summary).getByText("$3.20")).toBeInTheDocument();
  });

  it("renders model values without depending on horizontal table scrolling", () => {
    render(<UsageDashboard usage={usage} />);

    const modelRegion = screen.getByRole("region", { name: /usage by model/i });
    const table = within(modelRegion).getByRole("table", { name: "Usage by model" });

    for (const header of [
      "Model",
      "Included credits",
      "Additional credits",
      "Gross amount",
      "Additional usage",
      "Price per credit"
    ]) {
      expect(within(table).getByRole("columnheader", { name: header })).toBeInTheDocument();
    }

    const rows = within(table).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(within(rows[1]).getByRole("rowheader")).toHaveTextContent("gpt-4.1");
    expect(within(rows[1]).getAllByRole("cell").map((cell) => cell.textContent)).toEqual([
      "700",
      "80",
      "$7.80",
      "$0.80",
      "$0.01"
    ]);
    expect(within(rows[2]).getByRole("rowheader")).toHaveTextContent("claude-3.7-sonnet");
    expect(within(rows[2]).getAllByRole("cell").map((cell) => cell.textContent)).toEqual([
      "550",
      "240",
      "$7.90",
      "$2.40",
      "$0.01"
    ]);
    expect(document.querySelector(".table-wrap")).not.toBeInTheDocument();
  });

  it("renders one accessible stacked daily bar per day with exact tooltip data", () => {
    render(<UsageDashboard usage={usage} />);

    const daily = screen.getByRole("region", { name: /daily usage/i });
    expect(daily.querySelector(".daily-chart")).toHaveStyle("grid-template-columns: repeat(2, minmax(0, 1fr))");

    const bars = within(daily).getAllByRole("button", { name: /total credits.*additional usage/i });
    expect(bars).toHaveLength(2);
    expect(bars[0]).toHaveAccessibleName(
      "Jun 1: 900 included credits, 100 additional credits, 1,000 total credits, $1.00 additional usage"
    );
    expect(bars[1]).toHaveAccessibleName(
      "Jun 2: 350 included credits, 220 additional credits, 570 total credits, $2.20 additional usage"
    );
    expect(within(daily).getAllByText("Included credits").length).toBeGreaterThan(0);
    expect(within(daily).getAllByText("Additional credits").length).toBeGreaterThan(0);
    expect(within(daily).getAllByText("Total credits").length).toBeGreaterThan(0);
    expect(within(daily).queryByRole("tooltip")).not.toBeInTheDocument();

    fireEvent.focus(bars[1]);

    const selectedDay = within(daily).getByRole("region", { name: /selected day usage/i });
    expect(within(selectedDay).getByText("Jun 2")).toBeInTheDocument();
    expect(within(selectedDay).getByText("350")).toBeInTheDocument();
    expect(within(selectedDay).getByText("220")).toBeInTheDocument();
    expect(within(selectedDay).getByText("570")).toBeInTheDocument();
    expect(within(selectedDay).getByText("$2.20")).toBeInTheDocument();
  });

  it("does not show a cumulative budget limit without an explicit configured budget", () => {
    render(<UsageDashboard usage={usage} />);

    const daily = screen.getByRole("region", { name: /daily usage/i });
    const cumulativeMode = within(daily).getByRole("button", { name: "Cumulative" });

    fireEvent.click(cumulativeMode);

    expect(within(daily).queryByText(/100%.*limit/i)).not.toBeInTheDocument();
  });

  it("keeps full-month daily charts in one visible row without scrollbar sizing", () => {
    render(<UsageDashboard usage={usageWithDayCount(30)} />);

    const daily = screen.getByRole("region", { name: /daily usage/i });
    const chartFrame = daily.querySelector(".daily-chart-frame");

    expect(chartFrame).toBeInTheDocument();
    expect(chartFrame).not.toHaveAttribute("style");
    expect(daily.querySelector(".daily-chart")).toHaveStyle("grid-template-columns: repeat(30, minmax(0, 1fr))");
  });

  it("renders an available universal budget separately from credit totals", () => {
    render(
      <UsageDashboard
        usage={usageWithBudget({
          status: "available",
          source: "universal",
          monthlyLimitUsd: 30,
          consumedUsd: 12.5,
          remainingUsd: 17.5,
          usagePercent: 41.66666666666667,
          preventFurtherUsage: true
        })}
      />
    );

    const summary = screen.getByRole("complementary", { name: /usage summary/i });
    const budget = within(summary).getByRole("group", { name: "Monthly budget" });

    expect(within(budget).getByText("Monthly budget")).toBeInTheDocument();
    expect(within(budget).getByText("$30.00")).toBeInTheDocument();
    expect(within(budget).getByText("Used")).toBeInTheDocument();
    expect(within(budget).getByText("$12.50")).toBeInTheDocument();
    expect(within(budget).getByText("Remaining")).toBeInTheDocument();
    expect(within(budget).getByText("$17.50")).toBeInTheDocument();
    expect(within(budget).getByText("41.7% used")).toBeInTheDocument();
    expect(within(budget).getByText("Universal")).toBeInTheDocument();
    expect(within(budget).getByText("Usage stops at limit")).toBeInTheDocument();
    expect(within(budget).queryByText(/included credits/i)).not.toBeInTheDocument();
  });

  it("renders override, not-configured, and unavailable budget states", () => {
    const { rerender } = render(
      <UsageDashboard
        usage={usageWithBudget({
          status: "available",
          source: "override",
          monthlyLimitUsd: 50,
          consumedUsd: 12.5,
          remainingUsd: 37.5,
          usagePercent: 25,
          preventFurtherUsage: false
        })}
      />
    );

    let budget = within(screen.getByRole("complementary", { name: /usage summary/i })).getByRole("group", {
      name: "Monthly budget"
    });
    expect(within(budget).getByText("Override")).toBeInTheDocument();
    expect(within(budget).getByText("$50.00")).toBeInTheDocument();
    expect(within(budget).getByText("25% used")).toBeInTheDocument();
    expect(within(budget).queryByText("Usage stops at limit")).not.toBeInTheDocument();

    rerender(<UsageDashboard usage={usageWithBudget({ status: "not_configured" })} />);
    budget = within(screen.getByRole("complementary", { name: /usage summary/i })).getByRole("group", {
      name: "Monthly budget"
    });
    expect(within(budget).getByText("No user budget configured")).toBeInTheDocument();

    rerender(<UsageDashboard usage={usageWithBudget({ status: "unavailable" })} />);
    budget = within(screen.getByRole("complementary", { name: /usage summary/i })).getByRole("group", {
      name: "Monthly budget"
    });
    expect(within(budget).getByText("Budget unavailable")).toBeInTheDocument();
  });

  it("toggles the daily chart to cumulative usage with a converted budget line", () => {
    render(
      <UsageDashboard
        usage={usageWithBudget({
          status: "available",
          source: "universal",
          monthlyLimitUsd: 30,
          consumedUsd: 12.5,
          remainingUsd: 17.5,
          usagePercent: 41.66666666666667
        })}
      />
    );

    const daily = screen.getByRole("region", { name: /daily usage/i });
    const dailyMode = within(daily).getByRole("button", { name: "Daily" });
    const cumulativeMode = within(daily).getByRole("button", { name: "Cumulative" });

    expect(dailyMode).toHaveAttribute("aria-pressed", "true");
    expect(within(daily).queryByText("Budget")).not.toBeInTheDocument();

    fireEvent.click(cumulativeMode);

    expect(cumulativeMode).toHaveAttribute("aria-pressed", "true");
    expect(within(daily).getByText("Budget")).toBeInTheDocument();
    expect(within(daily).getByRole("note", { name: "Budget line: $30.00 monthly budget, 3,000 equivalent credits" })).toBeInTheDocument();
    expect(within(daily).queryByText("100% limit")).not.toBeInTheDocument();

    const chartList = within(daily).getByRole("list", { name: "Daily usage by day" });
    expect(within(chartList).queryByRole("note")).not.toBeInTheDocument();

    const bars = within(daily).getAllByRole("button", { name: /cumulative through/i });
    expect(bars).toHaveLength(2);
    expect(bars[1]).toHaveAccessibleName(
      "Cumulative through Jun 2: 1,250 included credits, 320 additional credits, 1,570 total credits, $3.20 additional usage"
    );

    fireEvent.focus(bars[1]);

    const selectedDay = within(daily).getByRole("region", { name: /selected cumulative usage/i });
    expect(within(selectedDay).getByText("Cumulative through")).toBeInTheDocument();
    expect(within(selectedDay).getByText("Jun 2")).toBeInTheDocument();
    expect(within(selectedDay).getByText("1,250")).toBeInTheDocument();
    expect(within(selectedDay).getByText("320")).toBeInTheDocument();
    expect(within(selectedDay).getByText("1,570")).toBeInTheDocument();
    expect(within(selectedDay).getByText("$3.20")).toBeInTheDocument();
  });

  it("switches and persists the theme preference", () => {
    render(<UsageDashboard usage={usage} />);

    const dark = screen.getByRole("button", { name: "Dark" });
    fireEvent.click(dark);

    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(dark).toHaveAttribute("aria-pressed", "true");
    expect(localStorage.setItem).toHaveBeenCalledWith("copilot-usage-theme", "dark");
  });

  it("renders partial empty states for missing daily or model usage", () => {
    render(<UsageDashboard usage={{ ...usage, daily: [], models: [] }} />);

    const daily = screen.getByRole("region", { name: /daily usage/i });
    expect(within(daily).getByText("No daily usage")).toBeInTheDocument();

    const table = screen.getByRole("table", { name: /usage by model/i });
    expect(within(table).getByText("No model usage")).toBeInTheDocument();
  });
});
