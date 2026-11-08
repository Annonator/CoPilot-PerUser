import type { NextAuthConfig } from "next-auth";
import { beforeEach, describe, expect, it, vi } from "vitest";

const nextAuthMocks = vi.hoisted(() => ({
  config: undefined as NextAuthConfig | undefined,
  auth: vi.fn(),
  get: vi.fn(),
  post: vi.fn(),
  signIn: vi.fn(),
  signOut: vi.fn()
}));

vi.mock("next-auth", () => ({
  default: vi.fn((config: NextAuthConfig) => {
    nextAuthMocks.config = config;
    return {
      handlers: {
        GET: nextAuthMocks.get,
        POST: nextAuthMocks.post
      },
      auth: nextAuthMocks.auth,
      signIn: nextAuthMocks.signIn,
      signOut: nextAuthMocks.signOut
    };
  })
}));

vi.mock("next-auth/providers/google", () => ({
  default: vi.fn((config) => ({ id: "google", ...config }))
}));

const validAuthSecret = "0123456789abcdef0123456789abcdef";

describe("auth secret validation", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
    nextAuthMocks.config = undefined;
    nextAuthMocks.auth.mockReset();
    nextAuthMocks.get.mockReset();
    nextAuthMocks.post.mockReset();
    nextAuthMocks.signIn.mockReset();
    nextAuthMocks.signOut.mockReset();
    nextAuthMocks.auth.mockResolvedValue(null);
    nextAuthMocks.get.mockResolvedValue(new Response(null));
    nextAuthMocks.post.mockResolvedValue(new Response(null));
    nextAuthMocks.signIn.mockResolvedValue(undefined);
    nextAuthMocks.signOut.mockResolvedValue(undefined);
  });

  it("rejects the public default AUTH_SECRET before returning a dev session", async () => {
    vi.stubEnv("AUTH_SECRET", "replace-with-random-auth-secret");
    vi.stubEnv("AUTH_DEV_EMAIL", "user@company.name");

    const { auth } = await import("./auth");

    await expect(auth()).rejects.toThrow(/AUTH_SECRET/);
    expect(nextAuthMocks.auth).not.toHaveBeenCalled();
  });

  it("rejects the public default AUTH_SECRET before invoking Auth.js route handlers", async () => {
    vi.stubEnv("AUTH_SECRET", "replace-with-random-auth-secret");

    const { handlers } = await import("./auth");
    const request = new Request("http://localhost/api/auth/session") as Parameters<
      typeof handlers.GET
    >[0];

    await expect(handlers.GET(request)).rejects.toThrow(/AUTH_SECRET/);
    expect(nextAuthMocks.get).not.toHaveBeenCalled();
  });

  it("initializes only validated OAuth sign-ins and never resets time on updates", async () => {
    vi.stubEnv("AUTH_SECRET", validAuthSecret);
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");
    vi.stubEnv("AUTH_DEV_EMAIL", "");
    vi.stubEnv("AUTH_REVOKED_GOOGLE_SUBJECTS", "");
    vi.stubEnv("AUTH_REVOKED_EMAILS", "");
    const { auth } = await import("./auth");
    await auth();
    const callback = nextAuthMocks.config!.callbacks!.jwt!;
    const account = { provider: "google", type: "oidc", providerAccountId: "google-123" };
    const profile = { sub: "google-123", email: "user@company.name", email_verified: true };
    const token = await callback({ token: {}, account, profile, trigger: "signIn" } as Parameters<typeof callback>[0]);
    expect(token).toMatchObject({ googleSubject: "google-123", email: "user@company.name" });
    expect(token?.authenticatedAt).toBeTypeOf("number");
    const updated = await callback({
      token,
      trigger: "update",
      session: { authenticatedAt: Number.MAX_SAFE_INTEGER, googleSubject: "different" }
    } as Parameters<typeof callback>[0]);
    expect(updated).toEqual(token);
    const invalid = await callback({
      token: {}, account, profile: { ...profile, email_verified: false }, trigger: "signIn"
    } as Parameters<typeof callback>[0]);
    expect(invalid).toBeNull();
    const legacy = await callback({ token: { email: "user@company.name" } } as Parameters<typeof callback>[0]);
    expect(legacy).toBeNull();
  });

  it("accepts a long AUTH_SECRET for local dev sessions", async () => {
    vi.stubEnv("AUTH_SECRET", validAuthSecret);
    vi.stubEnv("AUTH_DEV_EMAIL", "user@company.name");

    const { auth } = await import("./auth");

    await expect(auth()).resolves.toMatchObject({
      user: {
        email: "user@company.name"
      }
    });
  });
});
