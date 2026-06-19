import { describe, expect, it, vi } from "vitest";

import { isAllowedCompanyEmail, isVerifiedAllowedCompanyEmail } from "./company-email";

describe("isAllowedCompanyEmail", () => {
  it("allows exact emails with a configured domain case-insensitively", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isAllowedCompanyEmail("ana@Company.Name")).toBe(true);
  });

  it("allows any configured domain from a comma-separated list", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name,example.org");

    expect(isAllowedCompanyEmail("ana@example.org")).toBe(true);
  });

  it("rejects missing email", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isAllowedCompanyEmail(undefined)).toBe(false);
    expect(isAllowedCompanyEmail(null)).toBe(false);
    expect(isAllowedCompanyEmail("")).toBe(false);
  });

  it("rejects emails with multiple at signs", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isAllowedCompanyEmail("ana@company.name@example.org")).toBe(false);
  });

  it("rejects emails with an empty local part", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isAllowedCompanyEmail("@company.name")).toBe(false);
  });

  it("rejects whitespace-wrapped emails", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isAllowedCompanyEmail(" ana@company.name ")).toBe(false);
  });
});

describe("isVerifiedAllowedCompanyEmail", () => {
  it("allows a configured company email only when Google verified it", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isVerifiedAllowedCompanyEmail("ana@company.name", true)).toBe(true);
  });

  it("rejects unverified Google profile emails even when the domain matches", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isVerifiedAllowedCompanyEmail("ana@company.name", false)).toBe(false);
    expect(isVerifiedAllowedCompanyEmail("ana@company.name", "true")).toBe(false);
    expect(isVerifiedAllowedCompanyEmail("ana@company.name", undefined)).toBe(false);
  });

  it("rejects verified emails outside configured company domains", () => {
    vi.stubEnv("COMPANY_EMAIL_DOMAINS", "company.name");

    expect(isVerifiedAllowedCompanyEmail("ana@example.org", true)).toBe(false);
  });
});
