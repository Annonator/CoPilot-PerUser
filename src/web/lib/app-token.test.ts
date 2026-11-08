// @vitest-environment node

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppToken } from "./app-token";

const authMock = vi.hoisted(() => vi.fn());
vi.mock("@/auth", () => ({ auth: authMock }));

const validAppTokenSecret = "0123456789abcdef0123456789abcdef";

describe("createAppToken", () => {
  beforeEach(() => {
    authMock.mockReset().mockResolvedValue({ user: { email: "user@company.name" } });
  });
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("rejects the public default APP_TOKEN_SECRET before minting a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", "replace-with-random-app-token-secret");

    await expect(createAppToken()).rejects.toThrow(
      /APP_TOKEN_SECRET/
    );
  });

  it("rejects short APP_TOKEN_SECRET values before minting a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", "short-secret");

    await expect(createAppToken()).rejects.toThrow(
      /APP_TOKEN_SECRET/
    );
  });

  it("rejects a missing or denied session before minting a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", validAppTokenSecret);
    authMock.mockResolvedValue(null);
    await expect(createAppToken()).rejects.toThrow(/active session/);
    expect(authMock).toHaveBeenCalledOnce();
  });

  it("accepts a long APP_TOKEN_SECRET and mints a token", async () => {
    vi.stubEnv("APP_TOKEN_SECRET", validAppTokenSecret);

    await expect(createAppToken()).resolves.toMatch(
      /^[^.]+\.[^.]+\.[^.]+$/
    );
  });
});
