import "server-only";

import NextAuth from "next-auth";
import Google from "next-auth/providers/google";

import {
  firstConfiguredCompanyDomain,
  isAllowedCompanyEmail,
  isVerifiedAllowedCompanyEmail
} from "@/lib/company-email";
import { devSessionFromEnv } from "@/lib/dev-session";
import { requireServerSecret } from "@/lib/server-secret";

type NextAuthInstance = ReturnType<typeof NextAuth>;
type AuthRouteHandler = NextAuthInstance["handlers"]["GET"];

let nextAuth: NextAuthInstance | undefined;

export const handlers = {
  GET: async (...args: Parameters<AuthRouteHandler>) => {
    return getNextAuth().handlers.GET(...args);
  },
  POST: async (...args: Parameters<AuthRouteHandler>) => {
    return getNextAuth().handlers.POST(...args);
  }
};

export async function signIn(...args: Parameters<NextAuthInstance["signIn"]>) {
  return getNextAuth().signIn(...args);
}

export async function signOut(...args: Parameters<NextAuthInstance["signOut"]>) {
  return getNextAuth().signOut(...args);
}

export async function auth() {
  requireAuthSecret();
  return devSessionFromEnv() ?? getNextAuth().auth();
}

function hostedDomainHint(): string | undefined {
  return firstConfiguredCompanyDomain();
}

function requireAuthSecret(): string {
  return requireServerSecret("AUTH_SECRET");
}

function getNextAuth(): NextAuthInstance {
  const secret = requireAuthSecret();

  if (!nextAuth) {
    nextAuth = NextAuth({
      secret,
      trustHost: true,
      providers: [
        Google({
          clientId: process.env.AUTH_GOOGLE_ID,
          clientSecret: process.env.AUTH_GOOGLE_SECRET,
          authorization: {
            params: {
              hd: hostedDomainHint()
            }
          }
        })
      ],
      callbacks: {
        async signIn({ profile }) {
          return isVerifiedAllowedCompanyEmail(profile?.email, profile?.email_verified);
        }
      }
    });
  }

  return nextAuth;
}

export { isAllowedCompanyEmail };
