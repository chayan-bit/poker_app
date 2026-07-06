// Friends screen: real GET /api/friends (+presence) and GET /api/friends/pending,
// add-by-id/name, accept/decline, and rail-a-friend spectating. This is the
// top-level export the app router mounts (e.g. at "/friends"); it takes no
// required props and owns its own data fetching/polling.
//
// Presence is polled every 10s ONLY while this screen is mounted (interval is
// created and cleared inside a single effect here, not anywhere global).

import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card } from "@/components/ui/kit";
import { ApiError } from "@/net/api";
import { listFriends, listPendingFriends, type FriendEntry, type PendingEntry } from "@/net/social";
import { FriendRow } from "./FriendRow";
import { PendingRequests } from "./PendingRequests";
import { AddFriend } from "./AddFriend";
import { SpectatorTable } from "./SpectatorTable";

const PRESENCE_POLL_MS = 10_000;

export default function FriendsScreen() {
  const nav = useNavigate();
  const [friends, setFriends] = useState<FriendEntry[]>([]);
  const [pending, setPending] = useState<PendingEntry[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [railTableId, setRailTableId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [f, p] = await Promise.all([listFriends(), listPendingFriends()]);
      setFriends(f);
      setPending(p);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.message : "Could not load friends");
    }
  }, []);

  // Initial load + presence poll every 10s while mounted; cleared on unmount.
  useEffect(() => {
    void refresh();
    const id = window.setInterval(() => void refresh(), PRESENCE_POLL_MS);
    return () => window.clearInterval(id);
  }, [refresh]);

  if (railTableId) {
    return (
      <div className="h-full w-full">
        <SpectatorTable tableId={railTableId} onLeave={() => setRailTableId(null)} />
      </div>
    );
  }

  return (
    <Screen
      title="Friends"
      back={
        <button
          onClick={() => nav(-1)}
          className="grid h-9 w-9 place-items-center rounded-xl text-ink-dim no-tap-highlight"
          style={{ boxShadow: "inset 0 0 0 1px var(--line)" }}
          aria-label="Back"
        >
          <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
            <path d="M15 5 8 12l7 7" />
          </svg>
        </button>
      }
    >
      <Card>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-ink-dim">
          Add a friend
        </h2>
        <AddFriend onSent={() => void refresh()} />
      </Card>

      {pending.length > 0 && (
        <Card>
          <PendingRequests pending={pending} onResolved={() => void refresh()} />
        </Card>
      )}

      <Card>
        <h2 className="mb-1 text-sm font-semibold uppercase tracking-wide text-ink-dim">
          Friends
        </h2>
        {loadError && <p className="mb-2 text-xs text-ink-faint">{loadError}</p>}
        {friends.length === 0 && !loadError ? (
          <p className="py-2 text-sm text-ink-faint">No friends yet - add one above.</p>
        ) : (
          <ul className="flex flex-col divide-y divide-line">
            {friends.map((f) => (
              <FriendRow
                key={f.playerId}
                friend={f}
                onRail={setRailTableId}
                onRemoved={() => void refresh()}
              />
            ))}
          </ul>
        )}
      </Card>
    </Screen>
  );
}
