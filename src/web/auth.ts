import NextAuth from "next-auth";
import Google from "next-auth/providers/google";

function configuredDomains(): string[] {
  return (process.env.COMPANY_EMAIL_DOMAINS ?? "")
    .split(",")
    .map((domain) => domain.trim().toLowerCase())
    .filter(Boolean);
}

function emailDomain(email: string): string {
  return email.toLowerCase().split("@").at(1) ?? "";
}

export function isAllowedCompanyEmail(email: string | null | undefined): boolean {
  if (!email) {
    return false;
  }

  const domains = configuredDomains();
  return domains.length > 0 && domains.includes(emailDomain(email));
}

function hostedDomainHint(): string | undefined {
  return configuredDomains()[0];
}

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
    async signIn({ profile, user }) {
      return isAllowedCompanyEmail(profile?.email ?? user.email);
    }
  }
});
