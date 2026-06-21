import "server-only";

type ServerSecretName = "AUTH_SECRET" | "APP_TOKEN_SECRET";

const minimumSecretLength = 32;
const weakSecrets = new Set([
  "changeme",
  "change-me",
  "secret",
  "test",
  "password",
  "local-app-token-secret",
  "local-auth-secret",
  "build-secret"
]);

export function requireServerSecret(
  name: ServerSecretName,
  value: string | undefined = process.env[name]
): string {
  const secret = value?.trim();
  if (!secret) {
    throw new Error(`${name} is required`);
  }
  if (secret !== value) {
    throw new Error(`${name} must not include leading or trailing whitespace`);
  }

  const normalized = secret.toLowerCase();
  if (isPlaceholderSecret(normalized) || secret.length < minimumSecretLength) {
    throw new Error(`${name} must be a long random value; placeholders and short secrets are not allowed`);
  }

  return secret;
}

function isPlaceholderSecret(secret: string): boolean {
  return secret.includes("replace-with") || secret.includes("placeholder") || weakSecrets.has(secret);
}
