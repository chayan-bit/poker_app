// Friends list with presence (at a table / in lobby / offline), one-tap
// "join their table" and "rail them" (spectate).

import { memo } from "react";
import { useNavigate } from "react-router-dom";
import { Card } from "@/components/ui/kit";

type Presence = "table" | "lobby" | "offline";

interface Friend {
  id: string;
  name: string;
  presence: Presence;
  tableId?: string;
}

const FRIENDS: Friend[] = [
  { id: "1", name: "Nova", presence: "table", tableId: "AB12CD" },
  { id: "2", name: "Kaito", presence: "lobby" },
  { id: "3", name: "Priya", presence: "table", tableId: "ZX99QQ" },
  { id: "4", name: "Marlowe", presence: "offline" },
];

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

function FriendsPanelImpl() {
  const nav = useNavigate();
  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-ink-dim">
        Friends
      </h2>
      <ul className="flex flex-col divide-y divide-line">
        {FRIENDS.map((f) => (
          <li key={f.id} className="flex items-center gap-3 py-2.5">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ background: DOT[f.presence] }}
              aria-hidden
            />
            <div className="flex-1">
              <p className="font-medium">{f.name}</p>
              <p className="text-xs text-ink-faint">{LABEL[f.presence]}</p>
            </div>
            {f.presence === "table" && (
              <div className="flex gap-1.5">
                <button
                  className="rounded-lg border border-line px-2.5 py-1 text-xs"
                  onClick={() => nav(`/table?join=${f.tableId}`)}
                >
                  Join
                </button>
                <button
                  className="rounded-lg border border-line px-2.5 py-1 text-xs text-ink-dim"
                  onClick={() => nav(`/table?join=${f.tableId}&rail=1`)}
                >
                  Rail
                </button>
              </div>
            )}
          </li>
        ))}
      </ul>
    </Card>
  );
}

export const FriendsPanel = memo(FriendsPanelImpl);
