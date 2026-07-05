// Chip and big-blind formatting. All numeric UI uses these + the .num class so
// widths never jitter as counts tick.

export function formatChips(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(n % 1_000_000 ? 1 : 0) + "M";
  if (n >= 10_000) return (n / 1000).toFixed(n % 1000 ? 1 : 0) + "k";
  return n.toLocaleString("en-US");
}

/** Render an amount either as chips or as big blinds, per the display toggle. */
export function formatAmount(
  n: number,
  bb: number,
  inBB: boolean,
): string {
  if (inBB && bb > 0) {
    const v = n / bb;
    return (Number.isInteger(v) ? v.toString() : v.toFixed(1)) + " BB";
  }
  return formatChips(n);
}
