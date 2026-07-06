// Rail-a-friend: connects the same WS client the real table page uses,
// sends join_table for the friend's tableId WITHOUT sit_down, and renders the
// existing Table component read-only. Table already renders with no action
// bar and no hole cards whenever the store's yourSeat is null (a spectator
// never gets a seat_dealt/hand_dealt for their own seat), so no game-logic
// changes are needed here beyond that.
//
// This intentionally reuses the same store/transport singleton TablePage
// uses (one WS connection app-wide) rather than opening a second socket.

import { useEffect } from "react";
import { useGame } from "@/store/gameStore";
import { getStoredToken } from "@/net/api";
import { Cmd } from "@/net/protocol";
import Table from "@/components/table/Table";

interface Props {
  tableId: string;
  onLeave: () => void;
}

export function SpectatorTable({ tableId, onLeave }: Props) {
  const connect = useGame((s) => s.connect);
  const disconnect = useGame((s) => s.disconnect);
  const status = useGame((s) => s.status);

  useEffect(() => {
    const url = import.meta.env.VITE_WS_URL as string | undefined;
    const token = getStoredToken() ?? undefined;
    connect({ url, mock: !url, token });
    // Join as a spectator: no sit_down is ever sent, so the snapshot that
    // comes back keeps yourSeat === null.
    useGame.getState().transport?.send({
      type: Cmd.JoinTable,
      data: { tableId },
    });
    return () => disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tableId]);

  return (
    <div className="relative h-full w-full">
      <Table />
      <div className="absolute left-3 top-3 z-20 flex items-center gap-2">
        <button
          onClick={onLeave}
          className="glass no-tap-highlight rounded-full px-3.5 py-1.5 text-xs font-medium text-ink-dim"
        >
          Leave
        </button>
        {status !== "open" && (
          <span className="glass rounded-full px-3 py-1.5 text-xs text-ink-faint">
            {status === "closed" ? "Disconnected" : "Connecting…"}
          </span>
        )}
      </div>
    </div>
  );
}
