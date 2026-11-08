import "server-only";

import { SignJWT } from "jose";

import { auth } from "@/auth";

import { requireServerSecret } from "./server-secret";

const encoder = new TextEncoder();

export async function createAppToken(): Promise<string> {
  const secret = requireServerSecret("APP_TOKEN_SECRET");
  // Resolve the current session here so every mint checks absolute expiry and
  // revocation, even if a caller previously retained an authenticated email.
  const session = await auth();
  if (!session?.user?.email) {
    throw new Error("An active session is required to create an API token");
  }

  return new SignJWT({
    email: session.user.email,
    name: session.user.name ?? ""
  })
    .setProtectedHeader({ alg: "HS256", typ: "JWT" })
    .setExpirationTime("5m")
    .sign(encoder.encode(secret));
}
