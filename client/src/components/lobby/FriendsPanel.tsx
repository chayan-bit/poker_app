// Friends list with live presence (at a table / in lobby / offline), one-tap
// "join their table" and "rail them" (spectate). Data comes from the real
// social API (GET /api/friends); loading, empty and error states are all
// surfaced inline.

import { memo, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card } from "@/components/ui/kit";
import { ApiError } from "@/net/api";
import { listFriends, type FriendEntry } from "@/net/social";

type Presence = "offline" | "lobby" | "table";

const DOT: Record<Presence, string> = {
  table: "var(--gold)",
  lobby: "var(--action-blue)",
  offline: "var(--ink-faint)",
};

const LABEL: Record<Presence, string> = {
  table: "At a table",
  lobby: "In lobby",
  offline: "Offline",
};

type LoadState = "loading" | "ready" | "error";

function FriendsPanelImpl() {
  const nav = useNavigate();
  const [friends, setFriends] = useState<FriendEntry[]>([]);
  const [state, setState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    setState("loading");
    listFriends()
      .then((rows) => {
        if (!active) return;
        setFriends(rows);
        setState("ready");
      })
      .catch((err) => {
        if (!active) return;
        setError(err instanceof ApiError ? err.message : "Could not load friends.");
        setState("error");
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-ink-dim">
        Friends
      </h2>

      {state === "loading" && (
        <p className="text-sm text-ink-faint" role="status">
          Loading friends...
        </p>
      )}

      {state === "error" && (
        <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
          {error}
        </p>
      )}

      {state === "ready" && friends.length === 0 && (
        <p className="text-sm text-ink-faint">
          No friends yet. Add players from a table to see them here.
        </p>
      )}

      {state === "ready" && friends.length > 0 && (
        <ul className="flex flex-col divide-y divide-line">
          {friends.map((f) => {
            const presence = f.status.state as Presence;
            const atTable = presence === "table" && f.status.tableId.length > 0;
            return (
              <li key={f.playerId} className="flex items-center gap-3 py-2.5">
                <span
                  className="h-2.5 w-2.5 shrink-0 rounded-full"
                  style={{ background: DOT[presence] }}
                  aria-hidden
                />
                <div className="flex-1 min-w-0">
                  <p className="truncate font-medium">{f.name ?? f.playerId}</p>
                  <p className="text-xs text-ink-faint">{LABEL[presence]}</p>
                </div>
                {atTable && (
                  <div className="flex gap-1.5">
                    <button
                      className="rounded-lg border border-line px-2.5 py-1 text-xs"
                      onClick={() => nav(`/table?join=${encodeURIComponent(f.status.tableId)}`)}
                    >
                      Join
                    </button>
                    <button
                      className="rounded-lg border border-line px-2.5 py-1 text-xs text-ink-dim"
                      onClick={() =>
                        nav(`/table?join=${encodeURIComponent(f.status.tableId)}&rail=1`)
                      }
                    >
                      Rail
                    </button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

export const FriendsPanel = memo(FriendsPanelImpl);
