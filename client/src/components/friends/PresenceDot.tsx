// Presence indicator dot: gray offline, blue lobby, green at-table. Mirrors
// server/internal/social/presence.go's Status.State exactly (no client-side
// mapping table beyond color/label).

import type { PresenceStatus } from "@/net/social";

const DOT: Record<PresenceStatus["state"], string> = {
  offline: "var(--ink-faint)",
  lobby: "var(--action-blue)",
  table: "var(--success)",
};

const LABEL: Record<PresenceStatus["state"], string> = {
  offline: "Offline",
  lobby: "In lobby",
  table: "At a table",
};

export function PresenceDot({ state }: { state: PresenceStatus["state"] }) {
  return (
    <span
      className="h-2.5 w-2.5 shrink-0 rounded-full"
      style={{ background: DOT[state] }}
      aria-hidden
    />
  );
}

export function presenceLabel(state: PresenceStatus["state"]): string {
  return LABEL[state];
}
