// Final-standings overlay: broadcast once as tourney_result, shown to every
// remaining client. Gold treatment for 1st, Fraunces display headline, mono
// (tabular) numbers for places and prizes - never floats, chips are integers
// end to end. Routes back to the lobby on dismiss.

import { useNavigate } from "react-router-dom";
import { useGame } from "@/store/gameStore";
import { ordinal } from "./ordinal";

export function StandingsOverlay() {
  const result = useGame((s) => s.tourney.result);
  const disconnect = useGame((s) => s.disconnect);
  const clearResult = useGame((s) => s.clearTourneyResult);
  const nav = useNavigate();

  if (!result) return null;

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
                    {p.playerId}
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
