// Full-screen placement card shown to the hero the moment they bust out of a
// sit-and-go (before the tournament-wide standings, if any, land). Superseded
// by StandingsOverlay once tourney_result arrives (that overlay is the
// definitive final screen), so this only renders while result is still null.

import { useNavigate } from "react-router-dom";
import { useGame } from "@/store/gameStore";
import { ordinal } from "./ordinal";

export function PlacementScreen() {
  const myPlace = useGame((s) => s.tourney.myPlace);
  const result = useGame((s) => s.tourney.result);
  const disconnect = useGame((s) => s.disconnect);
  const nav = useNavigate();

  if (myPlace === null || result !== null) return null;

  const toLobby = () => {
    disconnect();
    nav("/lobby");
  };

  return (
    <div
      className="absolute inset-0 z-30 grid place-items-center px-6"
      style={{ background: "rgba(8,10,13,0.86)", backdropFilter: "blur(6px)" }}
      role="alertdialog"
      aria-modal="true"
    >
      <div
        className="card-edge flex w-full max-w-sm flex-col items-center gap-4 rounded-2xl p-7 text-center"
        style={{
          transform: "translateY(0) scale(1)",
          opacity: 1,
          animation: "placementIn var(--dur-slow) var(--ease)",
        }}
      >
        <p className="text-xs font-semibold uppercase tracking-[0.24em] text-ink-faint">
          Tournament over for you
        </p>
        <h1 className="display text-[2rem]">
          Finished <span className="display-accent">{ordinal(myPlace)}</span>
        </h1>
        <button
          onClick={toLobby}
          className="no-tap-highlight mt-2 min-h-[52px] w-full rounded-xl px-5 text-base font-semibold tracking-tight transition-transform duration-150 ease-out hover:-translate-y-[1px] active:translate-y-[1px]"
          style={{
            background: "var(--surface-4)",
            color: "var(--ink)",
            boxShadow: "var(--shadow-1), inset 0 1px 0 rgba(255,255,255,0.09), inset 0 0 0 1px var(--line-hi)",
          }}
        >
          Back to lobby
        </button>
      </div>
      <style>{`
        @keyframes placementIn {
          from { opacity: 0; transform: translateY(10px) scale(0.98); }
          to { opacity: 1; transform: translateY(0) scale(1); }
        }
      `}</style>
    </div>
  );
}
