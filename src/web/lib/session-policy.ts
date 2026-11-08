import "server-only";

import type { Account, Profile } from "next-auth";
import type { JWT } from "next-auth/jwt";

import { isAllowedCompanyEmail, isVerifiedAllowedCompanyEmail } from "./company-email";

// Auth.js refreshes iat/exp when reading a session. This separate timestamp is
// set only after a successful Google OAuth callback and never renewed by reads.
export const AUTH_SESSION_MAX_AGE_SECONDS = 60 * 60;

function validSubject(subject: unknown): subject is string {
  return typeof subject === "string" && subject.length > 0 && subject.trim() === subject;
}

function configuredList(value: string | undefined): string[] {
  return (value ?? "")
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export function isRevokedIdentity(subject: string | undefined, email: string): boolean {
  const subjects = configuredList(process.env.AUTH_REVOKED_GOOGLE_SUBJECTS);
  const emails = configuredList(process.env.AUTH_REVOKED_EMAILS);
  return (subject !== undefined && subjects.includes(subject)) ||
    emails.some((entry) => entry.toLowerCase() === email.toLowerCase());
}

export function isAllowedGoogleSignIn(account: Account | null | undefined, profile?: Profile): boolean {
  const email = profile?.email;
  const subject = profile?.sub;
  return account?.provider === "google" &&
    validSubject(subject) &&
    account.providerAccountId === subject &&
    typeof email === "string" &&
    isVerifiedAllowedCompanyEmail(email, profile?.email_verified) &&
    !isRevokedIdentity(subject, email);
}

export function createGoogleSessionToken(
  token: JWT,
  account: Account | null | undefined,
  profile?: Profile
): JWT | null {
  if (!profile || !isAllowedGoogleSignIn(account, profile)) {
    return null;
  }

  return {
    ...token,
    email: profile.email,
    googleSubject: profile.sub,
    authenticatedAt: Math.floor(Date.now() / 1000)
  };
}

export function isSessionTokenAllowed(token: JWT): boolean {
  const authenticatedAt = token.authenticatedAt;
  const now = Math.floor(Date.now() / 1000);
  return typeof authenticatedAt === "number" &&
    Number.isSafeInteger(authenticatedAt) &&
    authenticatedAt > 0 &&
    authenticatedAt <= now &&
    now - authenticatedAt < AUTH_SESSION_MAX_AGE_SECONDS &&
    validSubject(token.googleSubject) &&
    typeof token.email === "string" &&
    isAllowedCompanyEmail(token.email) &&
    !isRevokedIdentity(token.googleSubject, token.email);
}
