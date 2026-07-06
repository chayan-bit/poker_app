// One row in the hand-history list: hand id, table, timestamp, and the
// caller's potWon for that hand. Presentation only.

import type { ApiHandSummary } from "@/net/hands";
import { formatChips } from "@/lib/format";

function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export function HistoryRow({
  summary,
  onSelect,
}: {
  summary: ApiHandSummary;
  onSelect: (handId: string) => void;
}) {
  const won = summary.potWon > 0;
  return (
    <button
      onClick={() => onSelect(summary.handId)}
      className="no-tap-highlight flex w-full items-center justify-between gap-3 rounded-xl px-3 py-3 text-left transition-colors hover:bg-white/[0.03]"
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="truncate text-sm text-ink">Table {summary.tableId}</span>
        <span className="num text-xs text-ink-faint">
          {formatTimestamp(summary.startedAt)}
        </span>
      </div>
      <span
        className="num shrink-0 text-base font-semibold"
        style={{ color: won ? "#3fb27f" : "var(--ink-dim)" }}
      >
        {won ? "+" : ""}
        {formatChips(summary.potWon)}
      </span>
    </button>
  );
}
