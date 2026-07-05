// The active player's shrinking timebank ring. We compute the remaining
// fraction from the server-provided deadline (actByMs). To avoid a per-frame
// React re-render (the table must not re-render per frame), the ring is driven
// by a CSS transition on stroke-dashoffset, not by JS state. This hook only
// returns the target fraction + whether we're in the final-5s "urgent" window.

import { useEffect, useState } from "react";

export interface Timebank {
  /** 1 -> full, 0 -> expired */
  fraction: number;
  urgent: boolean;
  totalMs: number;
}

export function useTimebank(actByMs: number | null): Timebank {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (actByMs === null) return;
    // One coarse tick per second only to flip the urgent color; the smooth
    // shrink is a CSS transition. This is deliberately NOT rAF.
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [actByMs]);

  if (actByMs === null) return { fraction: 0, urgent: false, totalMs: 0 };

  const totalMs = 20_000; // display assumption; server is authoritative on expiry
  const remaining = Math.max(0, actByMs - now);
  const fraction = Math.max(0, Math.min(1, remaining / totalMs));
  return { fraction, urgent: remaining <= 5000, totalMs };
}
