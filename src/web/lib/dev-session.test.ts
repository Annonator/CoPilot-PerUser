import { describe, expect, it } from "vitest";

import { devSessionFromEnv } from "./dev-session";

describe("devSessionFromEnv", () => {
  it("returns a local session for AUTH_DEV_EMAIL outside production", () => {
    const session = devSessionFromEnv({
      AUTH_DEV_EMAIL: "user@company.name",
      AUTH_DEV_NAME: "Local User",
      NODE_ENV: "development"
    });

    expect(session).toMatchObject({
      user: {
        email: "user@company.name",
        name: "Local User"
      }
    });
    expect(session?.expires).toMatch(/^\d{4}-\d{2}-\d{2}T/);
  });

  it("returns null in production even when AUTH_DEV_EMAIL is set", () => {
    expect(
      devSessionFromEnv({
        AUTH_DEV_EMAIL: "user@company.name",
        NODE_ENV: "production"
      })
    ).toBeNull();
  });
});
