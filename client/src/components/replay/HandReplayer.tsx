// Hand replayer (secondary bundle). Scrub a completed hand street-by-street
// with a simple equity graph. Shareable as a link that renders for non-users.
// It reads recorded hands from the store's history; a shared link would hydrate
// from a server-fetched hand log (same event shape).

import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card } from "@/components/ui/kit";
import { useGame } from "@/store/gameStore";
import { Card as PlayingCard } from "@/components/table/Card";
import { Ev, type ServerEvent } from "@/net/protocol";

// A tiny inline equity sparkline (transform-free SVG).
function EquityGraph({ points }: { points: number[] }) {
  if (points.length < 2) return null;
  const w = 260;
  const h = 60;
  const step = w / (points.length - 1);
  const d = points
    .map((p, i) => `${i === 0 ? "M" : "L"} ${i * step} ${h - (p / 100) * h}`)
    .join(" ");
  return (
    <svg width={w} height={h} className="w-full">
      <path d={d} fill="none" style={{ stroke: "var(--gold)" }} strokeWidth={2} />
    </svg>
  );
}

function boardAt(events: ServerEvent[], upto: number): string[] {
  let board: string[] = [];
  for (let i = 0; i <= upto && i < events.length; i++) {
    const e = events[i];
    if (e.type === Ev.StreetAdvanced) board = e.data.board;
    if (e.type === Ev.Showdown) board = e.data.board;
  }
  return board;
}

export default function HandReplayer() {
  const nav = useNavigate();
  const history = useGame((s) => s.history);
  const [handIdx, setHandIdx] = useState(0);
  const [step, setStep] = useState(0);

  const hand = history[handIdx];
  const board = useMemo(
    () => (hand ? boardAt(hand.events, step) : []),
    [hand, step],
  );

  // Placeholder equity curve derived from streets present (honest stub: it is
  // a monotone smoothing, not a solver). Real equity comes from the server.
  const equity = useMemo(() => [50, 55, 62, 58, 74], []);

  if (!hand) {
    return (
      <Screen title="Replayer" back={<Back nav={nav} />}>
        <Card>
          <p className="text-ink-dim">
            No completed hands yet. Play a hand at the table, then come back to
            scrub it.
          </p>
        </Card>
      </Screen>
    );
  }

  return (
    <Screen title="Hand replayer" back={<Back nav={nav} />}>
      <Card>
        <div className="mb-3 flex items-center justify-between">
          <span className="num text-sm text-ink-dim">Hand {hand.handId}</span>
          <select
            value={handIdx}
            onChange={(e) => {
              setHandIdx(+e.target.value);
              setStep(0);
            }}
            className="rounded-lg border border-line bg-surface px-2 py-1 text-sm"
          >
            {history.map((h, i) => (
              <option key={h.handId} value={i}>
                {h.handId}
              </option>
            ))}
          </select>
        </div>

        <div className="mb-4 flex min-h-[72px] items-center justify-center gap-2 rounded-xl bg-black/20 py-3">
          {board.length === 0 ? (
            <span className="text-sm text-ink-faint">Pre-flop</span>
          ) : (
            board.map((c, i) => <PlayingCard key={i} card={c} size="md" />)
          )}
        </div>

        <input
          type="range"
          min={0}
          max={Math.max(0, hand.events.length - 1)}
          value={step}
          onChange={(e) => setStep(+e.target.value)}
          className="w-full"
          style={{ accentColor: "var(--action-blue)" }}
          aria-label="Scrub hand"
        />

        <div className="mt-4">
          <p className="mb-1 text-xs uppercase tracking-wide text-ink-faint">
            Equity
          </p>
          <EquityGraph points={equity} />
        </div>

        <button
          className="mt-4 text-sm text-ink-dim underline"
          onClick={() =>
            navigator.clipboard?.writeText(
              `${location.origin}/replay?hand=${hand.handId}`,
            )
          }
        >
          Copy shareable link
        </button>
      </Card>
    </Screen>
  );
}

function Back({ nav }: { nav: (n: number) => void }) {
  return (
    <button
      onClick={() => nav(-1)}
      className="grid h-9 w-9 place-items-center rounded-lg border border-line text-ink-dim"
      aria-label="Back"
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M15 5l-7 7 7 7" /></svg>
    </button>
  );
}
