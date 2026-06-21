import "server-only";

import { SignJWT } from "jose";

import { requireServerSecret } from "./server-secret";

type AppTokenSubject = {
  email: string;
  name?: string | null;
};

const encoder = new TextEncoder();

export async function createAppToken(subject: AppTokenSubject): Promise<string> {
  const secret = requireServerSecret("APP_TOKEN_SECRET");

  return new SignJWT({
    email: subject.email,
    name: subject.name ?? ""
  })
    .setProtectedHeader({ alg: "HS256", typ: "JWT" })
    .setExpirationTime("5m")
    .sign(encoder.encode(secret));
}
