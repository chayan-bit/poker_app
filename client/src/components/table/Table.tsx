// The poker table. ONE scene graph, two projections (portrait vertical /
// landscape oval) driven by container size. Seats, board and pot are persistent
// nodes that MOVE via transforms; we re-render per discrete event, never per
// frame. The felt is a stable backdrop: lit center, fractal-noise texture, a
// subtle walnut rail and rim light - a "card room at night", not a casino.

import { useEffect, useMemo, useRef, useState } from "react";
import { useGame } from "@/store/gameStore";
import { useSettings } from "@/store/settingsStore";
import { useNarration } from "@/hooks/useNarration";
import { formatAmount } from "@/lib/format";
import { computeLayout } from "./layout";
import { Seat } from "./Seat";
import { Board } from "./Board";
import { Pot } from "./Pot";
import { ActionBar } from "./ActionBar";
import { ReconnectBanner } from "./ReconnectBanner";
import { HostStart } from "./HostStart";
import { RebuyButton } from "./RebuyButton";
import { TableMenu } from "./TableMenu";
import type { BetBounds } from "./BetSlider";

export default function Table() {
  const wrapRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ w: 0, h: 0 });

  const seats = useGame((s) => s.seats);
  const pot = useGame((s) => s.pot);
  const board = useGame((s) => s.board);
  const blinds = useGame((s) => s.blinds);
  const buttonSeat = useGame((s) => s.buttonSeat);
  const yourSeat = useGame((s) => s.yourSeat);
  const yourHole = useGame((s) => s.yourHole);
  const nextToAct = useGame((s) => s.nextToAct);
  const actByMs = useGame((s) => s.actByMs);
  const street = useGame((s) => s.street);
  const showInBB = useSettings((s) => s.showInBB);
  const narration = useNarration();

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      setSize((s) => (s.w === width && s.h === height ? s : { w: width, h: height }));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const seatIndices = useMemo(
    () => seats.map((s) => s.seat).sort((a, b) => a - b),
    [seats],
  );

  const { slots, center } = useMemo(
    () => computeLayout({ width: size.w, height: size.h, seatIndices, heroSeat: yourSeat }),
    [size.w, size.h, seatIndices, yourSeat],
  );

  const bb = blinds[1] || 1;

  const { toCall, bounds } = useMemo(() => {
    const hero = seats.find((s) => s.seat === yourSeat);
    const maxBet = Math.max(
      0,
      ...seats.map((s) => (s.lastAction && s.lastAction.kind !== "fold" ? s.lastAction.amount : 0)),
    );
    const heroBet = hero?.lastAction && hero.lastAction.kind !== "fold" ? hero.lastAction.amount : 0;
    const call = Math.max(0, maxBet - heroBet);
    const stack = hero?.stack ?? 0;
    const b: BetBounds = {
      min: Math.min(stack, Math.max(bb, maxBet + bb)),
      max: stack,
      pot,
      bb,
    };
    return { toCall: Math.min(call, stack), bounds: b };
  }, [seats, yourSeat, bb, pot]);

  return (
    <div
      className="relative flex h-full w-full flex-col overflow-hidden"
      style={{
        background: "var(--surface)",
        // Clear the notch / Dynamic Island and rounded corners on device.
        paddingTop: "env(safe-area-inset-top)",
        paddingLeft: "env(safe-area-inset-left)",
        paddingRight: "env(safe-area-inset-right)",
      }}
    >
      <ReconnectBanner />

      <TopHud bb={bb} toCall={toCall} showInBB={showInBB} street={street} blinds={blinds} />

      {/* Felt scene */}
      <div ref={wrapRef} className="relative flex-1">
        {/* Rail (rim) */}
        <div
          className="absolute inset-2 rounded-[46%]"
          style={{
            background: "linear-gradient(160deg, var(--rail-hi), var(--rail))",
            boxShadow: "var(--shadow-3)",
          }}
        />
        {/* Felt */}
        <div
          className="gpu absolute inset-5 overflow-hidden rounded-[46%]"
          style={{
            background:
              "radial-gradient(115% 85% at 50% 40%, var(--felt-hi) 0%, var(--felt) 46%, var(--felt-edge) 100%)",
            boxShadow:
              "inset 0 0 70px rgba(0,0,0,0.5), inset 0 2px 0 rgba(255,255,255,0.06)",
            border: "1px solid rgba(0,0,0,0.4)",
          }}
        >
          {/* fractal-noise felt texture (self-contained SVG) */}
          <svg className="absolute inset-0 h-full w-full opacity-[0.05]" aria-hidden>
            <filter id="feltNoise">
              <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" stitchTiles="stitch" />
              <feColorMatrix type="saturate" values="0" />
            </filter>
            <rect width="100%" height="100%" filter="url(#feltNoise)" />
          </svg>
          {/* faint center brand monogram - printed on the felt, not typed */}
          <div
            className="absolute left-1/2 top-[36%] flex -translate-x-1/2 flex-col items-center gap-1.5"
            style={{ color: "rgba(255,255,255,0.10)" }}
          >
            <svg width={34} height={34} viewBox="0 0 48 48" fill="none" aria-hidden>
              <circle cx="24" cy="24" r="22.5" stroke="currentColor" strokeWidth="1.5" opacity="0.7" />
              <path
                d="M24 10c5.2 5 11 9.6 11 15.2 0 3.4-2.4 5.6-5.3 5.6-1.7 0-3.2-.8-4.2-2.1.4 2.6 1.5 5 3.5 6.6h-10c2-1.6 3.1-4 3.5-6.6-1 1.3-2.5 2.1-4.2 2.1-2.9 0-5.3-2.2-5.3-5.6C13 19.6 18.8 15 24 10Z"
                fill="currentColor"
              />
            </svg>
            <span className="display text-[10px] uppercase" style={{ letterSpacing: "0.42em" }}>
              Felt
            </span>
          </div>
        </div>

        {/* Center cluster: board + pot */}
        {center.x > 0 && (
          <div
            className="absolute flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-2"
            style={{ left: center.x, top: center.y }}
          >
            <Board board={board} />
            <Pot pot={pot} bb={bb} />
            <HostStart />
          </div>
        )}

        {/* Persistent seat nodes */}
        {slots.map((slot) => {
          const player = seats.find((s) => s.seat === slot.seat);
          if (!player) return null;
          const folded = player.lastAction?.kind === "fold";
          return (
            <Seat
              key={slot.seat}
              player={player}
              x={slot.x}
              y={slot.y}
              isHero={slot.seat === yourSeat}
              isButton={slot.seat === buttonSeat}
              isActive={slot.seat === nextToAct}
              folded={folded}
              winner={false}
              bb={bb}
              heroHole={slot.seat === yourSeat ? yourHole : []}
              actByMs={actByMs}
            />
          );
        })}
      </div>

      {/* Thumb-reachable action band */}
      <div className="shrink-0 border-t border-line" style={{ background: "var(--surface-2)" }}>
        <RebuyButton />
        <ActionBar toCall={toCall} bounds={bounds} />
      </div>

      <div className="sr-only" aria-live="polite" role="status">
        {narration}
      </div>
    </div>
  );
}

function TopHud({
  bb,
  toCall,
  showInBB,
  street,
  blinds,
}: {
  bb: number;
  toCall: number;
  showInBB: boolean;
  street: string | null;
  blinds: [number, number];
}) {
  const toggleBB = useSettings((s) => s.toggleShowInBB);
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2">
      <div className="glass flex items-center gap-2.5 rounded-full px-3 py-1.5 text-sm">
        <span className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {street ?? "-"}
        </span>
        <span className="h-3 w-px bg-line" />
        <span className="num text-ink-dim">
          Blinds {blinds[0]}/{blinds[1]}
        </span>
        {toCall > 0 && (
          <>
            <span className="h-3 w-px bg-line" />
            <span className="num font-semibold" style={{ color: "var(--action-blue-hi)" }}>
              Call {formatAmount(toCall, bb, showInBB)}
            </span>
          </>
        )}
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={toggleBB}
          className="glass rounded-full px-3 py-1.5 text-xs font-medium text-ink-dim no-tap-highlight"
        >
          {showInBB ? "BB" : "Chips"}
        </button>
        <TableMenu />
      </div>
    </div>
  );
}
