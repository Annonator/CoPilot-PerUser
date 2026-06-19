function configuredDomains(): string[] {
  return (process.env.COMPANY_EMAIL_DOMAINS ?? "")
    .split(",")
    .map((domain) => domain.trim().toLowerCase())
    .filter(Boolean);
}

export function firstConfiguredCompanyDomain(): string | undefined {
  return configuredDomains()[0];
}

export function isAllowedCompanyEmail(email: string | null | undefined): boolean {
  if (!email || email.trim() !== email) {
    return false;
  }

  const parts = email.split("@");
  if (parts.length !== 2 || !parts[0] || !parts[1]) {
    return false;
  }

  const domain = parts[1].toLowerCase();
  return configuredDomains().includes(domain);
}
