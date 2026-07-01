import "server-only";

const metadataIdentityURL =
  "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity";
const fallbackTokenTTLMS = 50 * 60 * 1000;
const tokenRefreshSkewMS = 5 * 60 * 1000;

type CachedToken = {
  audience: string;
  token: string;
  expiresAtMS: number;
};

let cachedToken: CachedToken | undefined;

export async function cloudRunIdentityAuthorizationHeader(): Promise<string | undefined> {
  const audience = process.env.API_ID_TOKEN_AUDIENCE?.trim();
  if (!audience) {
    return undefined;
  }

  const token = await fetchCloudRunIdentityToken(audience);
  return `Bearer ${token}`;
}

async function fetchCloudRunIdentityToken(audience: string): Promise<string> {
  const now = Date.now();
  if (
    cachedToken?.audience === audience &&
    cachedToken.expiresAtMS - tokenRefreshSkewMS > now
  ) {
    return cachedToken.token;
  }

  const url = new URL(metadataIdentityURL);
  url.searchParams.set("audience", audience);

  const response = await fetch(url, {
    headers: {
      "Metadata-Flavor": "Google"
    },
    cache: "no-store"
  });
  if (!response.ok) {
    throw new Error("Cloud Run identity token request failed.");
  }

  const token = (await response.text()).trim();
  if (!token) {
    throw new Error("Cloud Run identity token response was empty.");
  }

  cachedToken = {
    audience,
    token,
    expiresAtMS: tokenExpirationMS(token) ?? now + fallbackTokenTTLMS
  };
  return token;
}

function tokenExpirationMS(token: string): number | undefined {
  const [, payload] = token.split(".");
  if (!payload) {
    return undefined;
  }

  try {
    const claims = JSON.parse(Buffer.from(toBase64(payload), "base64").toString("utf8")) as {
      exp?: unknown;
    };
    return typeof claims.exp === "number" ? claims.exp * 1000 : undefined;
  } catch {
    return undefined;
  }
}

function toBase64(base64URL: string): string {
  const normalized = base64URL.replaceAll("-", "+").replaceAll("_", "/");
  return normalized.padEnd(normalized.length + ((4 - (normalized.length % 4)) % 4), "=");
}
