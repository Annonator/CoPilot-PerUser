import "server-only";

import { cloudRunIdentityAuthorizationHeader } from "./cloud-run-id-token";
import type { MonthlyUsage } from "./usage-types";

type GetUsageInput = {
  token: string;
  year: number;
  month: number;
};

export class UsageApiError extends Error {
  constructor(readonly status: number) {
    super("Usage API request failed.");
    this.name = "UsageApiError";
  }
}

export async function getMonthlyUsage({ token, year, month }: GetUsageInput): Promise<MonthlyUsage> {
  const baseUrl = process.env.API_BASE_URL;
  if (!baseUrl) {
    throw new Error("API_BASE_URL is required");
  }

  const url = new URL("/v1/usage", baseUrl);
  url.searchParams.set("year", String(year));
  url.searchParams.set("month", String(month));

  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
    Accept: "application/json"
  };
  const cloudRunAuthorization = await cloudRunIdentityAuthorizationHeader();
  if (cloudRunAuthorization) {
    headers["X-Serverless-Authorization"] = cloudRunAuthorization;
  }

  const response = await fetch(url, {
    headers,
    cache: "no-store"
  });

  if (!response.ok) {
    throw new UsageApiError(response.status);
  }

  return response.json() as Promise<MonthlyUsage>;
}
