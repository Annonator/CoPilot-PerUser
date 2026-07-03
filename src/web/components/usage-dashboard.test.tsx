import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

    expect(screen.getByText("Jun 2026")).toBeInTheDocument();
    expect(screen.getByText("@ana")).toBeInTheDocument();
    expect(screen.getByText("ana@company.name")).toBeInTheDocument();
    expect(screen.getByText("1,250")).toBeInTheDocument();
    expect(screen.getByText("320")).toBeInTheDocument();
    expect(screen.getByText("$15.70")).toBeInTheDocument();
    expect(screen.getByText("$3.20")).toBeInTheDocument();
  });

  it("renders model values without depending on horizontal table scrolling", () => {
    render(<UsageDashboard usage={usage} />);

    const modelRegion = screen.getByRole("region", { name: /usage by model/i });
    expect(within(modelRegion).getByText("gpt-4.1")).toBeInTheDocument();
    expect(within(modelRegion).getByText("claude-3.7-sonnet")).toBeInTheDocument();
    expect(within(modelRegion).getByText("Included credits")).toBeInTheDocument();
    expect(within(modelRegion).getByText("Additional credits")).toBeInTheDocument();
    expect(within(modelRegion).getByText("Gross amount")).toBeInTheDocument();
    expect(within(modelRegion).getByText("Additional usage")).toBeInTheDocument();
    expect(within(modelRegion).getByText("Price per credit")).toBeInTheDocument();
    expect(within(modelRegion).getByText("700")).toBeInTheDocument();
    expect(within(modelRegion).getByText("80")).toBeInTheDocument();
    expect(within(modelRegion).getByText("$7.80")).toBeInTheDocument();
    expect(within(modelRegion).getByText("$0.80")).toBeInTheDocument();
    expect(within(modelRegion).getAllByText("$0.01")).toHaveLength(2);
    expect(document.querySelector(".table-wrap")).not.toBeInTheDocument();
  });

  it("renders one accessible stacked daily bar per day with exact tooltip data", () => {
    render(<UsageDashboard usage={usage} />);

    const daily = screen.getByRole("region", { name: /daily usage/i });
    const bars = within(daily).getAllByRole("button");
    expect(bars).toHaveLength(2);
    expect(bars[0]).toHaveAccessibleName(
      "Jun 1: 900 included credits, 100 additional credits, 1,000 total credits, $1.00 additional usage"
    );
    expect(bars[1]).toHaveAccessibleName(
      "Jun 2: 350 included credits, 220 additional credits, 570 total credits, $2.20 additional usage"
    );
    expect(within(daily).getByText("Total credits")).toBeInTheDocument();
    expect(within(daily).getByText("1,000")).toBeInTheDocument();
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

    const table = screen.getByRole("table", { name: /model breakdown/i });
    expect(within(table).getByText("No model usage")).toBeInTheDocument();
  });
});
