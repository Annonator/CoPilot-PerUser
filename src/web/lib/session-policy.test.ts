import type { Account, Profile } from "next-auth";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createGoogleSessionToken, isAllowedGoogleSignIn } from "./session-policy";

const account: Account = { provider: "google", type: "oidc", providerAccountId: "google-sub-123" };
const profile: Profile = { sub: "google-sub-123", email: "user@company.name", email_verified: true };

describe("Google sign-in session policy", () => {
  beforeEach(() => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");
    vi.stubEnv("AUTH_REVOKED_GOOGLE_SUBJECTS", "");
    vi.stubEnv("AUTH_REVOKED_EMAILS", "");
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-08T12:00:00Z"));
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.useRealTimers();
  });

  it("sets an absolute timestamp and stable subject only for a verified allowed Google identity", () => {
    expect(isAllowedGoogleSignIn(account, profile)).toBe(true);
    expect(createGoogleSessionToken({ email: "stale@company.name", authenticatedAt: 1 }, account, profile)).toMatchObject({
      email: profile.email,
      googleSubject: profile.sub,
      authenticatedAt: Math.floor(Date.now() / 1000)
    });
  });

  it.each([
    { ...profile, email_verified: false },
    { ...profile, email_verified: "true" },
    { ...profile, email: "user@company.name.attacker.example" },
    { ...profile, sub: undefined },
    { ...profile, sub: "different-google-sub" },
    { ...profile, sub: " google-sub-123 " }
  ])("rejects unverified, disallowed or mismatched profiles (%j)", (invalidProfile) => {
    expect(isAllowedGoogleSignIn(account, invalidProfile as Profile)).toBe(false);
    expect(createGoogleSessionToken({}, account, invalidProfile as Profile)).toBeNull();
  });

  it("rejects another provider and missing OAuth state", () => {
    expect(isAllowedGoogleSignIn({ ...account, provider: "other" }, profile)).toBe(false);
    expect(createGoogleSessionToken({}, null, profile)).toBeNull();
    expect(createGoogleSessionToken({}, account)).toBeNull();
  });

  it.each([
    ["AUTH_REVOKED_GOOGLE_SUBJECTS", "google-sub-123"],
    ["AUTH_REVOKED_EMAILS", " USER@COMPANY.NAME "]
  ])("denies new sign-ins after %s revocation", (key, value) => {
    vi.stubEnv(key, value);
    expect(isAllowedGoogleSignIn(account, profile)).toBe(false);
    expect(createGoogleSessionToken({}, account, profile)).toBeNull();
  });
});
