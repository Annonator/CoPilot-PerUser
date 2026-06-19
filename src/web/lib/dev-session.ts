import type { Session } from "next-auth";

type DevSessionEnv = {
  AUTH_DEV_EMAIL?: string;
  AUTH_DEV_NAME?: string;
  NODE_ENV?: string;
};

export function devSessionFromEnv(env: DevSessionEnv = process.env): Session | null {
  const email = env.AUTH_DEV_EMAIL?.trim();
  if (!email || env.NODE_ENV === "production") {
    return null;
  }

  return {
    user: {
      email,
      name: env.AUTH_DEV_NAME?.trim() || "Local Demo User"
    },
    expires: new Date(Date.now() + 60 * 60 * 1000).toISOString()
  };
}
