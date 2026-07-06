// One row in the friends list: presence dot + name/status, a Rail button
// when the friend is at a table, and Remove. Rail failures (403/404 from the
// friend-table lookup) render as a quiet inline line under the row, never a
// modal - the caller owns that state since it's per-rail-attempt, not per-row.

import { useState } from "react";
import { ApiError } from "@/net/api";
import { getFriendTable, removeFriend, type FriendEntry } from "@/net/social";
import { PresenceDot, presenceLabel } from "./PresenceDot";

interface Props {
  friend: FriendEntry;
  onRail: (tableId: string) => void;
  onRemoved: () => void;
}

function railErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "not_friends") return "You are not friends";
    if (err.code === "not_at_table") return "Not at a table right now";
    return err.message;
  }
  return "Could not reach that table";
}

export function FriendRow({ friend, onRail, onRemoved }: Props) {
  const [railPending, setRailPending] = useState(false);
  const [railError, setRailError] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);

  async function rail() {
    setRailPending(true);
    setRailError(null);
    try {
      const { tableId } = await getFriendTable(friend.playerId);
      onRail(tableId);
    } catch (err) {
      setRailError(railErrorMessage(err));
    } finally {
      setRailPending(false);
    }
  }

  async function remove() {
    setRemoving(true);
    try {
      await removeFriend(friend.playerId);
      onRemoved();
    } finally {
      setRemoving(false);
    }
  }

  return (
    <li className="flex flex-col gap-1.5 py-2.5">
      <div className="flex items-center gap-3">
        <PresenceDot state={friend.status.state} />
        <div className="flex-1">
          <p className="font-medium">{friend.name || friend.playerId}</p>
          <p className="text-xs text-ink-faint">{presenceLabel(friend.status.state)}</p>
        </div>
        <div className="flex gap-1.5">
          {friend.status.state === "table" && (
            <button
              className="inline-flex min-h-[44px] items-center justify-center rounded-lg border border-line px-3.5 text-xs text-ink-dim no-tap-highlight disabled:opacity-50"
              disabled={railPending}
              onClick={() => void rail()}
            >
              {railPending ? "Railing…" : "Rail"}
            </button>
          )}
          <button
            className="inline-flex min-h-[44px] items-center justify-center rounded-lg border border-line px-3.5 text-xs text-ink-faint no-tap-highlight disabled:opacity-50"
            disabled={removing}
            onClick={() => void remove()}
          >
            Remove
          </button>
        </div>
      </div>
      {railError && <p className="pl-[1.375rem] text-xs text-ink-faint">{railError}</p>}
    </li>
  );
}
