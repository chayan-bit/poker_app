// Incoming friend requests with accept/decline, backed by
// GET /api/friends/pending and POST /api/friends/{accept,decline}.

import { useState } from "react";
import { ApiError } from "@/net/api";
import { acceptFriend, declineFriend, type PendingEntry } from "@/net/social";

interface Props {
  pending: PendingEntry[];
  onResolved: () => void;
}

export function PendingRequests({ pending, onResolved }: Props) {
  const [busyId, setBusyId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (pending.length === 0) return null;

  async function resolve(playerId: string, action: "accept" | "decline") {
    setBusyId(playerId);
    setError(null);
    try {
      if (action === "accept") await acceptFriend(playerId);
      else await declineFriend(playerId);
      onResolved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update that request");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-ink-dim">
        Pending requests
      </h3>
      <ul className="flex flex-col divide-y divide-line">
        {pending.map((p) => (
          <li key={p.playerId} className="flex items-center gap-3 py-2.5">
            <div className="flex-1">
              <p className="font-medium">{p.name || p.playerId}</p>
            </div>
            <div className="flex gap-1.5">
              <button
                className="rounded-lg border border-line px-2.5 py-1 text-xs disabled:opacity-50"
                disabled={busyId === p.playerId}
                onClick={() => void resolve(p.playerId, "accept")}
              >
                Accept
              </button>
              <button
                className="rounded-lg border border-line px-2.5 py-1 text-xs text-ink-dim disabled:opacity-50"
                disabled={busyId === p.playerId}
                onClick={() => void resolve(p.playerId, "decline")}
              >
                Decline
              </button>
            </div>
          </li>
        ))}
      </ul>
      {error && <p className="text-xs" style={{ color: "var(--danger)" }}>{error}</p>}
    </div>
  );
}
