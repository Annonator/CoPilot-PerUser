export type UsageTotals = {
  includedCredits: number;
  additionalCredits: number;
  grossAmount: number;
  additionalUsage: number;
};

export type ModelUsage = UsageTotals & {
  model: string;
  pricePerCredit: number;
};

export type DailyUsage = {
  day: string;
  totals: UsageTotals;
  models: ModelUsage[];
};

export type UserBudget = {
  status: "available" | "not_configured" | "unavailable";
  source?: "universal" | "override";
  budgetId?: string;
  parentBudgetId?: string;
  monthlyLimitUsd?: number;
  consumedUsd?: number;
  remainingUsd?: number;
  usagePercent?: number;
  preventFurtherUsage?: boolean;
};

export type MonthlyUsage = {
  period: {
    year: number;
    month: number;
  };
  user: {
    email: string;
    githubLogin: string;
  };
  totals: UsageTotals;
  budget?: UserBudget;
  daily: DailyUsage[];
  models: ModelUsage[];
  sourceMetadata: {
    enterprise: string;
    source: string;
    cached: boolean;
  };
};
