// Brief, non-blocking "blinds up" banner. Table chrome must not use Framer
// Motion, so the enter/exit is a hand-rolled transform/opacity transition
// driven by a mount-flag, matching the felt's other overlays.

import { useEffect, useState } from "react";
import { useGame } from "@/store/gameStore";

const VISIBLE_MS = 3200;

export function BlindsUpBanner() {
  const blindsUp = useGame((s) => s.tourney.blindsUp);
  const clear = useGame((s) => s.clearBlindsUpBanner);
  const [entered, setEntered] = useState(false);

  useEffect(() => {
    if (!blindsUp) return;
    setEntered(false);
    // Two rAFs so the initial (offscreen) frame paints before we animate in.
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
    // Keyed by `at` via the object identity swap in the store.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [blindsUp?.at]);

  if (!blindsUp) return null;

  return (
    <div
      className="pointer-events-none absolute left-1/2 top-3 z-20 -translate-x-1/2"
      style={{
        transform: `translate(-50%, 0) translateY(${entered ? 0 : -12}px)`,
        opacity: entered ? 1 : 0,
        transition: "transform var(--dur-slow) var(--ease), opacity var(--dur-slow) var(--ease)",
      }}
      role="status"
      aria-live="polite"
    >
      <div
        className="glass num flex items-center gap-2 rounded-full px-4 py-2 text-sm font-semibold"
        style={{ boxShadow: "var(--shadow-2), inset 0 0 0 1px var(--line-hi)", color: "var(--gold-hi)" }}
      >
        <BlindsGlyph />
        <span>
          Blinds up - {blindsUp.sb}/{blindsUp.bb} (level {blindsUp.level})
        </span>
      </div>
    </div>
  );
}

// Hand-drawn clock/ascending-bar glyph - never a Unicode symbol.
function BlindsGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.9} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M4 20V14M11 20V9M18 20V4" />
    </svg>
  );
}
