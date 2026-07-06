// Hand-history screen: the top-level export the app router mounts (e.g. at
// "/history"). Takes no required props and owns its own data fetching, like
// FriendsScreen. Lists the caller's recent hands (GET /api/players/me/hands),
// with load-more up to the server's cap (100), an empty state for new
// players, and a tap-to-open detail/replay view per hand.

import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card } from "@/components/ui/kit";
import { ApiError } from "@/net/api";
import { fetchMyHands, type ApiHandSummary } from "@/net/hands";
import { HistoryRow } from "./HistoryRow";
import { HandDetail } from "./HandDetail";

const PAGE_SIZE = 20;
const SERVER_MAX = 100;

export default function HistoryScreen() {
  const nav = useNavigate();
  const [hands, setHands] = useState<ApiHandSummary[]>([]);
  const [limit, setLimit] = useState(PAGE_SIZE);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);

  const load = useCallback(async (n: number) => {
    setLoading(true);
    try {
      const list = await fetchMyHands(n);
      setHands(list);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not load hand history");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(limit);
  }, [load, limit]);

  const canLoadMore = hands.length >= limit && limit < SERVER_MAX;

  return (
    <Screen title="Hand history" back={<Back nav={nav} />}>
      {selected && (
        <HandDetail handId={selected} onClose={() => setSelected(null)} />
      )}

      {!selected && (
        <>
          {error && (
            <Card>
              <p className="text-sm text-danger">{error}</p>
            </Card>
          )}

          {!error && !loading && hands.length === 0 && <EmptyState />}

          {hands.length > 0 && (
            <Card className="flex flex-col gap-1">
              {hands.map((h) => (
                <HistoryRow key={h.handId} summary={h} onSelect={setSelected} />
              ))}
            </Card>
          )}

          {loading && hands.length === 0 && (
            <Card>
              <p className="text-sm text-ink-faint">Loading…</p>
            </Card>
          )}

          {canLoadMore && (
            <button
              onClick={() => setLimit((n) => Math.min(SERVER_MAX, n + PAGE_SIZE))}
              disabled={loading}
              className="no-tap-highlight self-center text-sm text-ink-dim underline disabled:opacity-50"
            >
              {loading ? "Loading…" : "Load more"}
            </button>
          )}
        </>
      )}
    </Screen>
  );
}

function EmptyState() {
  return (
    <Card className="flex flex-col items-center gap-3 py-10 text-center">
      <svg
        width="40"
        height="40"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="text-ink-faint"
      >
        <rect x="4" y="3" width="16" height="18" rx="2" />
        <path d="M8 8h8M8 12h8M8 16h5" />
      </svg>
      <p className="display text-lg">No hands yet</p>
      <p className="max-w-xs text-sm text-ink-dim">
        Play a hand at a table and it will show up here, ready to replay or
        share.
      </p>
    </Card>
  );
}

function Back({ nav }: { nav: (n: number) => void }) {
  return (
    <button
      onClick={() => nav(-1)}
      className="grid h-9 w-9 place-items-center rounded-lg border border-line text-ink-dim"
      aria-label="Back"
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M15 5l-7 7 7 7" />
      </svg>
    </button>
  );
}
