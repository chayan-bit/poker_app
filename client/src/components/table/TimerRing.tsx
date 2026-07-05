// Shrinking ring around the active player's avatar. Driven by a CSS transition
// on stroke-dashoffset (not rAF) so the table never re-renders per frame. Color
// shifts to danger only in the final 5 seconds. No full-screen flashing.

import { memo } from "react";

interface Props {
  active: boolean;
  fraction: number; // 1 -> full, 0 -> empty
  urgent: boolean;
  size: number;
}

function TimerRingImpl({ active, fraction, urgent, size }: Props) {
  if (!active) return null;
  const stroke = 3;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const offset = c * (1 - fraction);
  return (
    <svg
      width={size}
      height={size}
      className="pointer-events-none absolute inset-0"
      style={{ transform: "rotate(-90deg)" }}
      aria-hidden
    >
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        stroke="rgba(255,255,255,0.08)"
        strokeWidth={stroke}
      />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        strokeWidth={stroke}
        strokeLinecap="round"
        strokeDasharray={c}
        strokeDashoffset={offset}
        style={{
          stroke: urgent ? "var(--danger)" : "var(--action-blue)",
          transition: "stroke-dashoffset 1s linear, stroke var(--dur) var(--ease)",
        }}
      />
    </svg>
  );
}

export const TimerRing = memo(TimerRingImpl);
