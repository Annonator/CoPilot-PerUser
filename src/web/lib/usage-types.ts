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
  daily: DailyUsage[];
  models: ModelUsage[];
  sourceMetadata: {
    enterprise: string;
    source: string;
    cached: boolean;
  };
};
