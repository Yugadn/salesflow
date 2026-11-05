/**
 * A fresh Idempotency-Key. crypto.randomUUID only exists in secure contexts
 * (https or localhost), so opening the app over plain http on a LAN address
 * falls back to getRandomValues, which works everywhere.
 */
export function newIdempotencyKey(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}
