// Seat-exit treatment for a busted opponent: a brief toast naming the seat
// and its finishing place. The hero's own elimination is handled by
// PlacementScreen instead (a full takeover, not a toast).

import { useEffect, useState } from "react";
import { useGame } from "@/store/gameStore";
import { ordinal } from "./ordinal";

const VISIBLE_MS = 2800;

export function EliminationToast() {
  const lastElimination = useGame((s) => s.tourney.lastElimination);
  const yourSeat = useGame((s) => s.yourSeat);
  const clear = useGame((s) => s.clearElimination);
  const [entered, setEntered] = useState(false);

  const forOpponent =
    lastElimination !== null && lastElimination.seat !== yourSeat;

  useEffect(() => {
    if (!forOpponent) return;
    setEntered(false);
    const raf = requestAnimationFrame(() =>
      requestAnimationFrame(() => setEntered(true)),
    );
    const hide = window.setTimeout(() => setEntered(false), VISIBLE_MS - 260);
    const dismiss = window.setTimeout(() => clear(), VISIBLE_MS);
    return () => {
      cancelAnimationFrame(raf);
      window.clearTimeout(hide);
      window.clearTimeout(dismiss);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lastElimination?.at, forOpponent]);

  if (!forOpponent || !lastElimination) return null;

  return (
    <div
      className="pointer-events-none absolute bottom-24 left-1/2 z-20 -translate-x-1/2"
      style={{
        transform: `translate(-50%, 0) translateY(${entered ? 0 : 10}px)`,
        opacity: entered ? 1 : 0,
        transition: "transform var(--dur-slow) var(--ease), opacity var(--dur-slow) var(--ease)",
      }}
      role="status"
      aria-live="polite"
    >
      <div
        className="glass flex items-center gap-2 rounded-full px-3.5 py-1.5 text-sm"
        style={{ boxShadow: "var(--shadow-1), inset 0 0 0 1px var(--line-hi)" }}
      >
        <span className="max-w-[8rem] truncate font-medium text-ink">
          {lastElimination.name}
        </span>
        <span className="num text-ink-dim">
          finished {ordinal(lastElimination.place)}
        </span>
      </div>
    </div>
  );
}
