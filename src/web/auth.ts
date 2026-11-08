import "server-only";

import NextAuth from "next-auth";
import Google from "next-auth/providers/google";

import {
  firstConfiguredCompanyDomain,
  isAllowedCompanyEmail
} from "@/lib/company-email";
import { devSessionFromEnv } from "@/lib/dev-session";
import { requireServerSecret } from "@/lib/server-secret";
import {
  AUTH_SESSION_MAX_AGE_SECONDS,
  createGoogleSessionToken,
  isAllowedGoogleSignIn,
  isRevokedIdentity,
  isSessionTokenAllowed
} from "@/lib/session-policy";

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
  const devSession = devSessionFromEnv();
  if (devSession?.user?.email) {
    return isRevokedIdentity(undefined, devSession.user.email) ? null : devSession;
  }
  return getNextAuth().auth();
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
      session: { strategy: "jwt", maxAge: AUTH_SESSION_MAX_AGE_SECONDS },
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
        async signIn({ account, profile }) {
          return isAllowedGoogleSignIn(account, profile);
        },
        async jwt({ token, account, profile, trigger }) {
          if (trigger === "signIn" || trigger === "signUp") {
            return createGoogleSessionToken(token, account, profile);
          }
          // Never copy client-provided session update data into the token.
          return isSessionTokenAllowed(token) ? token : null;
        },
        async session({ session, token }) {
          // Keep the subject and authentication timestamp inside the encrypted
          // cookie; the public session exposes only the usual user fields.
          return {
            ...session,
            expires: new Date(
              ((token.authenticatedAt as number) + AUTH_SESSION_MAX_AGE_SECONDS) * 1000
            ).toISOString()
          };
        }
      }
    });
  }

  return nextAuth;
}

export { isAllowedCompanyEmail };
