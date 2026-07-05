// Poker chips, hand-drawn as self-contained SVG (no external assets - crisp at
// any DPI, zero network fragility, bundles cleanly into Capacitor). A single
// chip is the classic disc with a dashed edge ring; a ChipStack renders a
// denomination-broken pile so a bet/pot reads as real chips, not just a number.

import { memo } from "react";

// Casino-standard denomination colors, mapped to our token palette where it
// helps legibility on the dark felt.
interface Denom {
  value: number;
  base: string;
  edge: string;
}

const DENOMS: Denom[] = [
  { value: 1000, base: "#2b2f36", edge: "#e8b44c" }, // black/gold
  { value: 500, base: "#6d28d9", edge: "#c4b5fd" }, // purple
  { value: 100, base: "#1b232a", edge: "#dfe6ec" }, // charcoal
  { value: 25, base: "#188a5a", edge: "#9df0c6" }, // green
  { value: 5, base: "#c0392b", edge: "#f3b0a6" }, // red
  { value: 1, base: "#e6edf2", edge: "#9fb0bd" }, // white
];

/** Break an amount into at most `max` chip denominations, largest first. */
export function breakIntoChips(amount: number, max = 5): Denom[] {
  const out: Denom[] = [];
  let rem = Math.max(0, Math.round(amount));
  for (const d of DENOMS) {
    while (rem >= d.value && out.length < max) {
      out.push(d);
      rem -= d.value;
    }
    if (out.length >= max) break;
  }
  if (out.length === 0) out.push(DENOMS[DENOMS.length - 1]);
  return out;
}

/** A single chip face, viewed slightly from above (an ellipse). */
function ChipDisc({ d, size }: { d: Denom; size: number }) {
  const r = size / 2;
  return (
    <g>
      <ellipse cx={r} cy={r} rx={r} ry={r * 0.82} fill="rgba(0,0,0,0.35)" />
      <ellipse
        cx={r}
        cy={r * 0.94}
        rx={r}
        ry={r * 0.82}
        fill={d.base}
        stroke="rgba(0,0,0,0.4)"
        strokeWidth={0.5}
      />
      {/* edge dashes */}
      <ellipse
        cx={r}
        cy={r * 0.94}
        rx={r * 0.82}
        ry={r * 0.67}
        fill="none"
        stroke={d.edge}
        strokeWidth={r * 0.13}
        strokeDasharray={`${r * 0.34} ${r * 0.28}`}
      />
      {/* inner disc */}
      <ellipse
        cx={r}
        cy={r * 0.94}
        rx={r * 0.55}
        ry={r * 0.45}
        fill={d.base}
        stroke={d.edge}
        strokeWidth={0.8}
      />
    </g>
  );
}

interface StackProps {
  amount: number;
  size?: number;
  /** override chip count (for compact tags) */
  maxChips?: number;
}

/** A stacked pile that grows upward; the tallest single-color runs merge. */
export const ChipStack = memo(function ChipStack({
  amount,
  size = 30,
  maxChips = 5,
}: StackProps) {
  const chips = breakIntoChips(amount, maxChips);
  const step = size * 0.22; // vertical offset per chip
  const h = size * 0.82 + step * (chips.length - 1) + size * 0.12;
  return (
    <svg
      width={size}
      height={h}
      viewBox={`0 0 ${size} ${h}`}
      className="gpu overflow-visible"
      aria-hidden
    >
      {chips.map((d, i) => (
        <g
          key={i}
          transform={`translate(0, ${h - size * 0.94 - i * step})`}
          style={{ filter: i === chips.length - 1 ? "brightness(1.08)" : "none" }}
        >
          <ChipDisc d={d} size={size} />
        </g>
      ))}
    </svg>
  );
});

/** A single flat chip glyph for inline use (e.g. the pot pill dot). */
export const Chip = memo(function Chip({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden>
      <ChipDisc d={DENOMS[0]} size={size} />
    </svg>
  );
});
