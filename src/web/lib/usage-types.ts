export type UsageTotals = {
  includedCredits: number;
  additionalCredits: number;
  grossAmount: number;
  additionalUsage: number;
  pricePerCredit?: number;
};

export type ModelUsage = UsageTotals & {
  model: string;
};

export type DailyModelUsage = Omit<ModelUsage, "pricePerCredit">;

export type DailyUsage = {
  day: string;
  totals: UsageTotals;
  models?: DailyModelUsage[];
};

export type MonthlyUsage = {
  period: {
    year: number;
    month: number;
    label?: string;
  };
  user: {
    email: string;
    githubLogin?: string;
  };
  totals: UsageTotals;
  daily: DailyUsage[];
  models: ModelUsage[];
  sourceMetadata?: {
    cached?: boolean;
    generatedAt?: string;
    [key: string]: unknown;
  };
};
