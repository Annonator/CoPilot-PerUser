import type { MonthlyUsage } from "./usage-types";

type GetUsageInput = {
  token: string;
  year: number;
  month: number;
};

export async function getMonthlyUsage({ token, year, month }: GetUsageInput): Promise<MonthlyUsage> {
  const baseUrl = process.env.API_BASE_URL;
  if (!baseUrl) {
    throw new Error("API_BASE_URL is required");
  }

  const url = new URL("/v1/usage", baseUrl);
  url.searchParams.set("year", String(year));
  url.searchParams.set("month", String(month));

  const response = await fetch(url, {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json"
    },
    cache: "no-store"
  });

  if (!response.ok) {
    const detail = await response.text();
    throw new Error(detail || `Usage API returned ${response.status}`);
  }

  return response.json() as Promise<MonthlyUsage>;
}
