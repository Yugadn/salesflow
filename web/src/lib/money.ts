// Money arrives from the API as a decimal string ("96000", "1333.33"). Doing
// arithmetic on it as a JavaScript number would reintroduce the float errors
// the backend avoids, so amounts are handled as integer cents (bigint) and
// only converted to a number at the very end, for display.

export type Cents = bigint;

/** Parses a decimal string with at most 2 fraction digits into cents. */
export function toCents(amount: string): Cents {
  const trimmed = amount.trim();
  const match = /^(-?)(\d+)(?:\.(\d{1,2}))?$/.exec(trimmed);
  if (!match) throw new Error(`not a money amount: "${amount}"`);
  const [, sign, whole, fraction = ""] = match;
  const cents = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, "0"));
  return sign === "-" ? -cents : cents;
}

/**
 * Formats cents in the API's decimal form, which (like shopspring/decimal)
 * drops trailing zeros: "96000", "1333.33", "4200.5".
 */
export function centsToDecimal(cents: Cents): string {
  const negative = cents < 0n;
  const abs = negative ? -cents : cents;
  const whole = abs / 100n;
  const fraction = abs % 100n;
  const text = fraction === 0n ? whole.toString() : `${whole}.${fraction.toString().padStart(2, "0").replace(/0$/, "")}`;
  return negative ? `-${text}` : text;
}

/** unitPrice × qty, exactly. */
export function lineTotal(unitPrice: string, qty: number): Cents {
  return toCents(unitPrice) * BigInt(qty);
}

const idr = new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 0, maximumFractionDigits: 2 });

/** Formats a decimal string or cents as Rupiah, e.g. "Rp 96.000" or "Rp 1.333,33". */
export function formatIDR(amount: string | Cents): string {
  const cents = typeof amount === "bigint" ? amount : toCents(amount);
  // Safe: rupiah amounts stay far below 2^53 cents.
  return idr.format(Number(cents) / 100);
}

/** True when the string is a valid price input (non-negative, ≤ 2 decimals). */
export function isValidPrice(input: string): boolean {
  return /^\d+(\.\d{1,2})?$/.test(input.trim());
}
