import "server-only";

import NextAuth from "next-auth";
import Google from "next-auth/providers/google";

import {
  firstConfiguredCompanyDomain,
  isAllowedCompanyEmail,
  isVerifiedAllowedCompanyEmail
} from "@/lib/company-email";

export const { handlers, auth, signIn, signOut } = NextAuth({
  secret: process.env.AUTH_SECRET,
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

function hostedDomainHint(): string | undefined {
  return firstConfiguredCompanyDomain();
}

export { isAllowedCompanyEmail };
