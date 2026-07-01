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

  it("keeps the portable app token auth flow when Cloud Run IAM is not configured", async () => {
    vi.stubEnv("API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({ period: { year: 2026, month: 6 } })
    );
    vi.stubGlobal("fetch", fetchMock);

    await getMonthlyUsage({ token: "app-token", year: 2026, month: 6 });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0];
    expect(init?.headers).toMatchObject({
      Authorization: "Bearer app-token",
      Accept: "application/json"
    });
    expect(init?.headers).not.toHaveProperty("X-Serverless-Authorization");
  });

  it("adds a Cloud Run IAM identity token when an API audience is configured", async () => {
    vi.stubEnv("API_BASE_URL", "https://api.example.test");
    vi.stubEnv("API_ID_TOKEN_AUDIENCE", "https://api.example.test");
    const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
      const url = String(input);
      if (url.startsWith("http://metadata.google.internal/")) {
        expect(init?.headers).toMatchObject({ "Metadata-Flavor": "Google" });
        expect(url).toContain("audience=https%3A%2F%2Fapi.example.test");
        return new Response("google-id-token", { status: 200 });
      }
      return jsonResponse({ period: { year: 2026, month: 6 } });
    });
    vi.stubGlobal("fetch", fetchMock);

    await getMonthlyUsage({ token: "app-token", year: 2026, month: 6 });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [, apiInit] = fetchMock.mock.calls[1];
    expect(apiInit?.headers).toMatchObject({
      Authorization: "Bearer app-token",
      "X-Serverless-Authorization": "Bearer google-id-token",
      Accept: "application/json"
    });
  });
});

function jsonResponse(value: unknown): Response {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { "Content-Type": "application/json" }
  });
}
