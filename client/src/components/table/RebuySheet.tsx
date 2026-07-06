// Rebuy sheet: shown when the hero's seat stack is 0 and no hand is in
// flight. A bottom sheet (not a modal over the felt - rebuy only appears
// between hands, so a brief backdrop never blocks gameplay). Amount is
// bounded by table stakes (min = one big blind, max = 1000 big blinds,
// mirroring server/internal/table/session.go handleRebuy). Ledger errors
// (e.g. insufficient funds) render inline here, never as an alert/modal.

import { memo, useEffect, useState } from "react";
import { useGame } from "@/store/gameStore";
import { formatAmount } from "@/lib/format";

interface Props {
  open: boolean;
  onClose: () => void;
}

function RebuySheetImpl({ open, onClose }: Props) {
  const blinds = useGame((s) => s.blinds);
  const rebuy = useGame((s) => s.rebuy);
  const rebuyPending = useGame((s) => s.rebuyPending);
  const rebuyError = useGame((s) => s.rebuyError);
  const clearRebuyError = useGame((s) => s.clearRebuyError);

  const bb = blinds[1] || 1;
  const min = bb;
  const max = 1000 * bb;
  const [amount, setAmount] = useState(max);

  useEffect(() => {
    if (open) setAmount(max);
  }, [open, max]);

  if (!open) return null;

  const step = bb;
  const clamp = (v: number) => Math.max(min, Math.min(max, v));

  return (
    <div className="absolute inset-0 z-40 flex items-end justify-center" role="dialog" aria-label="Rebuy">
      <div
        className="absolute inset-0"
        style={{ background: "rgba(6,9,12,0.55)" }}
        onClick={() => {
          clearRebuyError();
          onClose();
        }}
      />
      <div
        className="relative z-10 w-full max-w-xl rounded-t-3xl p-5"
        style={{ background: "var(--surface-2)", boxShadow: "var(--shadow-3)" }}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 className="display text-[1.3rem]">Rebuy</h2>
          <button
            onClick={() => {
              clearRebuyError();
              onClose();
            }}
            className="grid h-9 w-9 place-items-center rounded-lg text-ink-dim no-tap-highlight"
            style={{ boxShadow: "inset 0 0 0 1px var(--line)" }}
            aria-label="Close"
          >
            <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round">
              <path d="M6 6l12 12M18 6 6 18" />
            </svg>
          </button>
        </div>

        <p className="mb-3 text-sm text-ink-dim">
          Table range {formatAmount(min, bb, false)} - {formatAmount(max, bb, false)}
        </p>

        <div className="mb-4 flex items-center gap-3">
          <button
            onClick={() => setAmount((a) => clamp(a - step))}
            className="no-tap-highlight grid h-11 w-11 shrink-0 place-items-center rounded-xl text-lg font-semibold"
            style={{ background: "var(--surface-4)", color: "var(--ink)" }}
            aria-label="Decrease"
          >
            -
          </button>
          <input
            type="range"
            min={min}
            max={max}
            step={step}
            value={amount}
            onChange={(e) => setAmount(clamp(Number(e.target.value)))}
            className="h-2 flex-1 cursor-pointer appearance-none rounded-full"
            style={{ accentColor: "var(--action-blue)", background: "var(--surface-3)" }}
            aria-label="Rebuy amount"
          />
          <button
            onClick={() => setAmount((a) => clamp(a + step))}
            className="no-tap-highlight grid h-11 w-11 shrink-0 place-items-center rounded-xl text-lg font-semibold"
            style={{ background: "var(--surface-4)", color: "var(--ink)" }}
            aria-label="Increase"
          >
            +
          </button>
        </div>

        <div className="num mb-4 text-center text-2xl font-bold" style={{ color: "var(--action-blue)" }}>
          {formatAmount(amount, bb, false)}
        </div>

        {rebuyError && (
          <p className="mb-3 rounded-lg px-3 py-2 text-sm" style={{ background: "rgba(232,92,92,0.12)", color: "var(--danger)" }}>
            {rebuyError}
          </p>
        )}

        <button
          onClick={() => rebuy(amount)}
          disabled={rebuyPending}
          className="no-tap-highlight min-h-[52px] w-full rounded-xl text-base font-semibold tracking-tight transition-transform active:translate-y-[1px] disabled:opacity-60"
          style={{
            background: "var(--gold)",
            color: "#231704",
            boxShadow:
              "var(--shadow-2), inset 0 1px 0 rgba(255,255,255,0.35), inset 0 -1px 0 rgba(0,0,0,0.18)",
          }}
        >
          {rebuyPending ? "Rebuying..." : "Rebuy"}
        </button>
      </div>
    </div>
  );
}

export const RebuySheet = memo(RebuySheetImpl);
