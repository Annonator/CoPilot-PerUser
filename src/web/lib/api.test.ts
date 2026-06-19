import { afterEach, describe, expect, it, vi } from "vitest";

import { getMonthlyUsage, UsageApiError } from "./api";

describe("getMonthlyUsage", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
  });

  it("does not expose raw non-OK response bodies in thrown errors", async () => {
    vi.stubEnv("API_BASE_URL", "https://api.example.test");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("raw token or upstream detail", { status: 502 }))
    );

    await expect(getMonthlyUsage({ token: "app-token", year: 2026, month: 6 })).rejects.toMatchObject(
      {
        message: "Usage API request failed.",
        status: 502
      }
    );
    await expect(getMonthlyUsage({ token: "app-token", year: 2026, month: 6 })).rejects.toBeInstanceOf(
      UsageApiError
    );
  });
});
