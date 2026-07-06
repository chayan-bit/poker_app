// Final-standings overlay: broadcast once as tourney_result, shown to every
// remaining client. Gold treatment for 1st, Fraunces display headline, mono
// (tabular) numbers for places and prizes - never floats, chips are integers
// end to end. Routes back to the lobby on dismiss.

import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useGame } from "@/store/gameStore";
import { ordinal } from "./ordinal";

/** TourneyPlace carries only a playerId; the wire has no display name on it.
 *  We map it through the seat name map (the only name source the client holds).
 *  When a finisher's seat is already gone (busted earlier, so no name is known)
 *  we show a short humanized handle, never the raw opaque id.
 *  SEAM: a proper fix would have the server include the display name on
 *  TourneyPlace, or the client would keep a playerId->name cache across the
 *  whole tournament rather than only the currently-seated players. */
function humanizeId(id: string): string {
  if (!id) return "Player";
  if (id.length <= 12) return id;
  return `Player ${id.slice(-4).toUpperCase()}`;
}

export function StandingsOverlay() {
  const result = useGame((s) => s.tourney.result);
  const seats = useGame((s) => s.seats);
  const lastElimination = useGame((s) => s.tourney.lastElimination);
  const disconnect = useGame((s) => s.disconnect);
  const clearResult = useGame((s) => s.clearTourneyResult);
  const nav = useNavigate();

  // Build a best-effort playerId -> name map from every name source the client
  // still holds at tournament end.
  const nameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const s of seats) {
      if (s.playerId && s.name?.trim()) m.set(s.playerId, s.name);
    }
    if (lastElimination?.playerId && lastElimination.name?.trim()) {
      m.set(lastElimination.playerId, lastElimination.name);
    }
    return m;
  }, [seats, lastElimination]);

  if (!result) return null;

  const displayName = (playerId: string): string =>
    nameById.get(playerId) ?? humanizeId(playerId);

  const toLobby = () => {
    clearResult();
    disconnect();
    nav("/lobby");
  };

  const places = [...result.places].sort((a, b) => a.place - b.place);

  return (
    <div
      className="absolute inset-0 z-40 grid place-items-center px-6"
      style={{ background: "rgba(8,10,13,0.9)", backdropFilter: "blur(8px)" }}
      role="alertdialog"
      aria-modal="true"
    >
      <div className="card-edge flex w-full max-w-sm flex-col gap-5 rounded-2xl p-7">
        <div className="text-center">
          <p className="text-xs font-semibold uppercase tracking-[0.24em] text-ink-faint">
            Sit &amp; go complete
          </p>
          <h1 className="display mt-1 text-[1.9rem]">
            Final <span className="display-accent">standings</span>
          </h1>
        </div>

        <div className="flex flex-col gap-2">
          {places.map((p) => {
            const gold = p.place === 1;
            return (
              <div
                key={p.playerId}
                className="flex items-center justify-between rounded-xl px-3.5 py-2.5"
                style={{
                  background: gold ? "color-mix(in oklab, var(--gold), transparent 88%)" : "var(--surface-3)",
                  boxShadow: gold ? "0 0 0 1px var(--gold)" : "inset 0 0 0 1px var(--line-hi)",
                }}
              >
                <div className="flex items-center gap-2.5">
                  <span
                    className="num grid h-7 w-7 place-items-center rounded-full text-xs font-bold"
                    style={{
                      background: gold ? "var(--gold)" : "var(--surface-4)",
                      color: gold ? "#231704" : "var(--ink-dim)",
                    }}
                  >
                    {p.place}
                  </span>
                  <span className="max-w-[9rem] truncate text-sm font-medium">
                    {displayName(p.playerId)}
                  </span>
                </div>
                <span
                  className="num text-sm font-semibold"
                  style={{ color: gold ? "var(--gold-hi)" : "var(--ink-dim)" }}
                >
                  {ordinal(p.place)} · {p.prize.toLocaleString("en-US")}
                </span>
              </div>
            );
          })}
        </div>

        <button
          onClick={toLobby}
          className="no-tap-highlight min-h-[52px] w-full rounded-xl px-5 text-base font-semibold tracking-tight transition-transform duration-150 ease-out hover:-translate-y-[1px] active:translate-y-[1px]"
          style={{
            background: "var(--gold)",
            color: "#231704",
            boxShadow: "var(--shadow-2), inset 0 1px 0 rgba(255,255,255,0.35), inset 0 -1px 0 rgba(0,0,0,0.18)",
          }}
        >
          Back to lobby
        </button>
      </div>
    </div>
  );
}
