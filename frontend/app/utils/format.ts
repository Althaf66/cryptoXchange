/**
 * Formats an amount for display.
 *
 * Values at or above 1 truncate to 2 decimals, so a balance reads 9334571.93
 * rather than 9334571.93906672. Truncation rather than rounding: it never
 * displays more available funds than a user actually has.
 *
 * Values below 1 keep their precision. A flat 2-decimal rule would render a
 * 0.0062 BTC size as 0.00 - the same trap formatNum guards against in
 * cmd/engine/engine.go.
 */
export function formatAmount(n?: number): string {
  if (n == null || !Number.isFinite(n) || n === 0) return "0";
  if (Math.abs(n) >= 1) return (Math.trunc(n * 100) / 100).toFixed(2);
  return String(Number(n.toFixed(8)));
}

/**
 * Same float-noise trim, but keeps every decimal - for values that become an
 * order quantity rather than something a user only reads. Display truncation
 * here would mean the 100% button could not sell a 10.0062 balance in full.
 */
export function formatPrecise(n: number): string {
  if (!Number.isFinite(n)) return "0";
  return String(Number(n.toFixed(8)));
}
