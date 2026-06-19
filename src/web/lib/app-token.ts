import "server-only";

import { SignJWT } from "jose";

type AppTokenSubject = {
  email: string;
  name?: string | null;
};

const encoder = new TextEncoder();

export async function createAppToken(subject: AppTokenSubject): Promise<string> {
  const secret = process.env.APP_TOKEN_SECRET;
  if (!secret) {
    throw new Error("APP_TOKEN_SECRET is required");
  }

  return new SignJWT({
    email: subject.email,
    name: subject.name ?? ""
  })
    .setProtectedHeader({ alg: "HS256", typ: "JWT" })
    .setExpirationTime("5m")
    .sign(encoder.encode(secret));
}
