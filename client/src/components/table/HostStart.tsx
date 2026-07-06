// Host-start control for private rooms waiting on the first hand. Renders in
// the felt center cluster: the host sees the ONE loud action in this state
// (a Start Game button); everyone else sees quiet "waiting" text. Server is
// authoritative - this only sends start_hand and renders table_status.
//
// Deviation: the wire's table_status event (server: tableStatus{WaitingForHost,
// SeatedCount}) does not carry a host player id, and no other event exposes
// Table.Cfg.HostPlayerID to the client (server code is out of scope for this
// change). Host detection is therefore a client-side heuristic that mirrors
// the server's own fallback rule (table/handlers.go: "first player to sit
// becomes host if the lobby did not set one") - the lowest occupied seat
// number is treated as the host seat.

import { memo } from "react";
import { useGame } from "@/store/gameStore";

function HostStartImpl() {
  const tableStatus = useGame((s) => s.tableStatus);
  const seats = useGame((s) => s.seats);
  const yourSeat = useGame((s) => s.yourSeat);
  const startHand = useGame((s) => s.startHand);

  if (!tableStatus || !tableStatus.waitingForHost) return null;

  const hostSeat = seats.reduce<number | null>(
    (min, s) => (min === null || s.seat < min ? s.seat : min),
    null,
  );
  const isHost = hostSeat !== null && hostSeat === yourSeat;

  return (
    <div className="flex flex-col items-center gap-2.5">
      {isHost ? (
        <button
          onClick={startHand}
          className="no-tap-highlight min-h-[52px] rounded-xl px-6 text-base font-semibold tracking-tight transition-transform duration-150 ease-out active:translate-y-[1px] active:scale-[0.99]"
          style={{
            background: "var(--gold)",
            color: "#231704",
            boxShadow:
              "var(--shadow-2), inset 0 1px 0 rgba(255,255,255,0.35), inset 0 -1px 0 rgba(0,0,0,0.18)",
          }}
        >
          Start Game
        </button>
      ) : (
        <p className="text-sm text-ink-dim">
          Waiting for host - {tableStatus.seatedCount} seated
        </p>
      )}
    </div>
  );
}

export const HostStart = memo(HostStartImpl);
