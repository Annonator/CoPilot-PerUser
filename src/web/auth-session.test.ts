// @vitest-environment node

import { NextRequest } from "next/server";
import { decode, encode } from "next-auth/jwt";
import { jwtVerify } from "jose";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const requestHeaders = vi.hoisted(() => ({ current: new Headers() }));
vi.mock("next/headers", () => ({
  headers: async () => requestHeaders.current
}));

const secret = "0123456789abcdef0123456789abcdef";
const cookieName = "authjs.session-token";
const start = new Date("2026-11-08T12:00:00Z");
const subject = "google-user-123";

async function sessionCookie(overrides: Record<string, unknown> = {}) {
  const token = await encode({
    secret,
    salt: cookieName,
    maxAge: 24 * 60 * 60,
    token: {
      sub: subject,
      email: "user@company.name",
      name: "Company User",
      googleSubject: subject,
      authenticatedAt: start.getTime() / 1000,
      ...overrides
    }
  });
  return `${cookieName}=${token}`;
}

async function readSession(cookie: string) {
  const { handlers } = await import("./auth");
  return handlers.GET(new NextRequest("http://localhost:3000/api/auth/session", {
    headers: { cookie }
  }));
}

function refreshedCookie(response: Response) {
  const cookie = response.headers.getSetCookie().find((value) => value.startsWith(`${cookieName}=`));
  expect(cookie).toBeDefined();
  return cookie!.split(";")[0];
}

describe("Auth.js session lifetime and revocation", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(start);
    vi.stubEnv("AUTH_SECRET", secret);
    vi.stubEnv("APP_TOKEN_SECRET", secret);
    vi.stubEnv("AUTH_GOOGLE_ID", "test-client");
    vi.stubEnv("AUTH_GOOGLE_SECRET", "test-secret");
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    vi.stubEnv("AUTH_DEV_EMAIL", "");
    vi.stubEnv("AUTH_REVOKED_GOOGLE_SUBJECTS", "");
    vi.stubEnv("AUTH_REVOKED_EMAILS", "");
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");
    requestHeaders.current = new Headers({ host: "localhost:3000" });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllEnvs();
  });

  it("keeps the absolute sign-in time across real session route refreshes", async () => {
    let cookie = await sessionCookie();
    for (const minutes of [20, 40, 59]) {
      vi.setSystemTime(start.getTime() + minutes * 60 * 1000);
      const response = await readSession(cookie);
      expect(response.status).toBe(200);
      expect(await response.json()).toEqual({
        user: { email: "user@company.name", name: "Company User" },
        expires: new Date(start.getTime() + 60 * 60 * 1000).toISOString()
      });
      cookie = refreshedCookie(response);
      const token = await decode({ secret, salt: cookieName, token: cookie.split("=")[1] });
      expect(token?.authenticatedAt).toBe(start.getTime() / 1000);
    }

    vi.setSystemTime(start.getTime() + 60 * 60 * 1000);
    const expired = await readSession(cookie);
    expect(await expired.json()).toBeNull();
    expect(refreshedCookie(expired)).toBe(`${cookieName}=`);
    requestHeaders.current.set("cookie", cookie);
    const { createAppToken } = await import("./lib/app-token");
    await expect(createAppToken()).rejects.toThrow(/active session/);
  });

  it("ignores client update data in the real POST session route", async () => {
    const cookie = await sessionCookie();
    vi.setSystemTime(start.getTime() + 59 * 60 * 1000);
    const { handlers } = await import("./auth");
    const csrf = await handlers.GET(new NextRequest("http://localhost:3000/api/auth/csrf"));
    const { csrfToken } = await csrf.json();
    const csrfCookie = csrf.headers.getSetCookie().map((value) => value.split(";")[0]).join("; ");
    const updated = await handlers.POST(new NextRequest("http://localhost:3000/api/auth/session", {
      method: "POST",
      headers: { cookie: `${cookie}; ${csrfCookie}`, "content-type": "application/json" },
      body: JSON.stringify({ csrfToken, data: {
        authenticatedAt: Math.floor(Date.now() / 1000),
        googleSubject: "different-user",
        email: "different@company.name"
      } })
    }));
    expect(updated.status).toBe(200);
    expect(await updated.json()).toMatchObject({ user: { email: "user@company.name" } });
    const updatedCookie = refreshedCookie(updated);
    const token = await decode({ secret, salt: cookieName, token: updatedCookie.split("=")[1] });
    expect(token).toMatchObject({ googleSubject: subject, authenticatedAt: start.getTime() / 1000 });
    vi.setSystemTime(start.getTime() + 60 * 60 * 1000);
    expect(await (await readSession(updatedCookie)).json()).toBeNull();
  });

  it.each([
    undefined, null, "1794139200", 0, -1, 1.5, Number.MAX_SAFE_INTEGER,
    start.getTime() / 1000 + 1,
    start.getTime() / 1000 - 60 * 60
  ])("rejects missing, malformed, future or expired authenticatedAt (%s)", async (authenticatedAt) => {
    const cookie = await sessionCookie({ authenticatedAt });
    expect(await (await readSession(cookie)).json()).toBeNull();
    requestHeaders.current.set("cookie", cookie);
    const { createAppToken } = await import("./lib/app-token");
    await expect(createAppToken()).rejects.toThrow(/active session/);
  });

  it.each([undefined, "", " google-user-123 "])("rejects a missing or malformed stable subject (%s)", async (googleSubject) => {
    expect(await (await readSession(await sessionCookie({ googleSubject }))).json()).toBeNull();
  });

  it.each([
    ["AUTH_REVOKED_GOOGLE_SUBJECTS", `another-user, ${subject}`],
    ["AUTH_REVOKED_EMAILS", "another@company.name, USER@COMPANY.NAME"]
  ])("rejects an existing session and token mint after %s revocation", async (key, value) => {
    const cookie = await sessionCookie();
    expect(await (await readSession(cookie)).json()).toMatchObject({ user: { email: "user@company.name" } });
    vi.stubEnv(key, value);
    expect(await (await readSession(cookie)).json()).toBeNull();
    requestHeaders.current.set("cookie", cookie);
    const { createAppToken } = await import("./lib/app-token");
    await expect(createAppToken()).rejects.toThrow(/active session/);
  });

  it("mints a five-minute API token for a current allowed Google session", async () => {
    requestHeaders.current.set("cookie", await sessionCookie());
    const { createAppToken } = await import("./lib/app-token");
    const { payload } = await jwtVerify(await createAppToken(), new TextEncoder().encode(secret));
    expect(payload).toMatchObject({ email: "user@company.name", name: "Company User", exp: start.getTime() / 1000 + 300 });
    expect(payload).not.toHaveProperty("googleSubject");
    expect(payload).not.toHaveProperty("authenticatedAt");
  });

  it("preserves development mode and applies email revocation before token minting", async () => {
    vi.stubEnv("NODE_ENV", "development");
    vi.stubEnv("AUTH_DEV_EMAIL", "demo@company.name");
    const { createAppToken } = await import("./lib/app-token");
    const { payload } = await jwtVerify(await createAppToken(), new TextEncoder().encode(secret));
    expect(payload.email).toBe("demo@company.name");
    vi.stubEnv("AUTH_REVOKED_EMAILS", "DEMO@COMPANY.NAME");
    await expect(createAppToken()).rejects.toThrow(/active session/);
  });

  it("ignores development mode in production", async () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("AUTH_DEV_EMAIL", "demo@company.name");
    const { createAppToken } = await import("./lib/app-token");
    await expect(createAppToken()).rejects.toThrow(/active session/);
  });
});
