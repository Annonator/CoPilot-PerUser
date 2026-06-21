// @vitest-environment node

import { afterEach, describe, expect, it, vi } from "vitest";

import { createAppToken } from "./app-token";

const validAppTokenSecret = "0123456789abcdef0123456789abcdef";

describe("createAppToken", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("rejects the public default APP_TOKEN_SECRET before minting a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", "replace-with-random-app-token-secret");

    await expect(createAppToken({ email: "user@company.name" })).rejects.toThrow(
      /APP_TOKEN_SECRET/
    );
  });

  it("rejects short APP_TOKEN_SECRET values before minting a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", "short-secret");

    await expect(createAppToken({ email: "user@company.name" })).rejects.toThrow(
      /APP_TOKEN_SECRET/
    );
  });

  it("accepts a long APP_TOKEN_SECRET and mints a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", validAppTokenSecret);

    await expect(createAppToken({ email: "user@company.name" })).resolves.toMatch(
      /^[^.]+\.[^.]+\.[^.]+$/
    );
  });
});
