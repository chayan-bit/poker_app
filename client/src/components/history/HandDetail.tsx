// Detail view for one hand: board + showdown, a mono-font text-export
// preview with a copy-to-clipboard share action, and a "Verify this hand"
// deep link into the fairness verifier.
//
// This does NOT render via the existing <HandReplayer/>: that component reads
// its data exclusively from `useGame(s => s.history)` (a zustand store slice
// populated by the live socket) and accepts no props, so an
// externally-fetched record has nowhere to go without a small store/prop
// addition outside this module's ownership. `mapRecord.ts` already produces
// the exact `HandRecord` shape that store slice holds, so wiring is a single
// store action away (see the top-level report). Until then, this view is a
// self-contained read of the fetched record via the same board-rendering
// primitives (<PlayingCard/>) so hand detail is still fully usable today.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card as UiCard, Button } from "@/components/ui/kit";
import { Card as PlayingCard } from "@/components/table/Card";
import { formatChips } from "@/lib/format";
import { fetchHand, fetchHandText, type ApiHandRecord } from "@/net/hands";
import { ApiError } from "@/net/api";
import { mapToHoleCards, mapToHandRecord } from "./mapRecord";
import { useGame } from "@/store/gameStore";

export function HandDetail({
  handId,
  onClose,
}: {
  handId: string;
  onClose: () => void;
}) {
  const nav = useNavigate();
  const loadReplayHand = useGame((s) => s.loadReplayHand);
  const [record, setRecord] = useState<ApiHandRecord | null>(null);
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [showText, setShowText] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setRecord(null);
    setError(null);
    fetchHand(handId)
      .then((rec) => {
        if (!cancelled) setRecord(rec);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : "Could not load hand");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [handId]);

  const copyText = async () => {
    const body = text ?? (await fetchHandText(handId));
    setText(body);
    await navigator.clipboard?.writeText(body);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  const revealText = async () => {
    const body = text ?? (await fetchHandText(handId));
    setText(body);
    setShowText((v) => !v);
  };

  const verify = () => {
    if (!record) return;
    nav("/fair", {
      state: { commitment: record.Commitment, seed: record.SeedHex },
    });
  };

  return (
    <UiCard className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <span className="num text-sm text-ink-dim">Hand {handId}</span>
        <button onClick={onClose} className="text-sm text-ink-dim underline">
          Close
        </button>
      </div>

      {error && <p className="text-sm text-danger">{error}</p>}

      {!record && !error && <p className="text-sm text-ink-faint">Loading…</p>}

      {record && (
        <>
          <div className="flex min-h-[72px] items-center justify-center gap-2 rounded-xl bg-black/20 py-3">
            {record.Board.length === 0 ? (
              <span className="text-sm text-ink-faint">No board (folded pre-flop)</span>
            ) : (
              record.Board.map((c, i) => <PlayingCard key={i} card={c} size="md" />)
            )}
          </div>

          <div className="flex flex-col gap-2">
            <p className="text-xs uppercase tracking-wide text-ink-faint">Showdown</p>
            {record.Seats.map((seat) => {
              const holeCards = mapToHoleCards(seat.Hole);
              const result = record.Results[String(seat.SeatID)];
              return (
                <div key={seat.SeatID} className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-1">
                    {holeCards.map((c, i) => (
                      <PlayingCard key={i} card={c} size="sm" />
                    ))}
                    <span className="num ml-2 text-sm text-ink-dim">
                      Seat {seat.SeatID}
                    </span>
                  </div>
                  <span className="text-xs text-ink-faint">{result ?? ""}</span>
                </div>
              );
            })}
            {record.Awards.length > 0 && (
              <p className="num text-sm text-ink-dim">
                Pot: {formatChips(record.Awards.reduce((s, a) => s + a.Amount, 0))}
              </p>
            )}
          </div>

          <div className="flex flex-wrap gap-2">
            <Button variant="ghost" onClick={copyText}>
              {copied ? "Copied" : "Copy hand text"}
            </Button>
            <Button variant="ghost" onClick={revealText}>
              {showText ? "Hide text" : "View text"}
            </Button>
            <Button variant="ghost" onClick={verify}>
              Verify this hand
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                loadReplayHand(mapToHandRecord(record));
                nav("/replay");
              }}
            >
              Replay
            </Button>
          </div>

          {showText && text && (
            <pre className="mono max-h-64 overflow-auto whitespace-pre-wrap rounded-xl bg-black/20 p-3 text-xs text-ink-dim">
              {text}
            </pre>
          )}
        </>
      )}
    </UiCard>
  );
}
