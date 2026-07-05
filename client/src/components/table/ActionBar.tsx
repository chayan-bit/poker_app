// The action bar lives in the thumb-reachable bottom band. Its buttons are the
// largest elements on the mobile screen. Tapping any action updates the UI
// immediately (optimistic) with NO spinner in the action path; the store tags
// it pending until the server confirms or rolls back. A rollback triggers a
// subtle shake here - never a modal.

import { memo, useEffect, useRef, useState } from "react";
import { useGame } from "@/store/gameStore";
import { useSettings } from "@/store/settingsStore";
import { formatAmount } from "@/lib/format";
import { BetSlider, presetAmount, type BetBounds } from "./BetSlider";
import { useTableKeys } from "@/hooks/useTableKeys";

interface Props {
  toCall: number;
  bounds: BetBounds;
}

function ActionBarImpl({ toCall, bounds }: Props) {
  const act = useGame((s) => s.act);
  const isMyTurn = useGame((s) => s.nextToAct === s.yourSeat && s.yourSeat !== null);
  const pending = useGame((s) => s.pending);
  const rollbackNonce = useGame((s) => s.rollbackNonce);
  const showInBB = useSettings((s) => s.showInBB);
  const presets = useSettings((s) => s.presets);

  const [amount, setAmount] = useState(bounds.min);
  const [raiseOpen, setRaiseOpen] = useState(false);
  const barRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setAmount((a) => Math.max(bounds.min, Math.min(bounds.max, a || bounds.min)));
  }, [bounds.min, bounds.max]);

  useEffect(() => {
    if (rollbackNonce === 0) return;
    const el = barRef.current;
    if (!el) return;
    el.classList.remove("rollback-shake");
    void el.offsetWidth;
    el.classList.add("rollback-shake");
  }, [rollbackNonce]);

  const canCheck = toCall <= 0;
  const doFold = () => act("fold", 0);
  const doCheckCall = () => (canCheck ? act("check", 0) : act("call", toCall));
  const doRaise = () => {
    if (!raiseOpen) {
      setRaiseOpen(true);
      return;
    }
    act(toCall > 0 ? "raise" : "bet", amount);
    setRaiseOpen(false);
  };

  useTableKeys({
    enabled: isMyTurn && !pending,
    onFold: doFold,
    onCheckCall: doCheckCall,
    onRaise: doRaise,
    onPreset: (i) => {
      const p = presets[i];
      if (p) setAmount(presetAmount(p, bounds));
    },
  });

  if (!isMyTurn && !pending) {
    return (
      <div className="flex items-center justify-center gap-2 px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-3 text-sm text-ink-faint">
        <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
        Waiting for the action
      </div>
    );
  }

  const raiseLabel = toCall > 0 ? "Raise" : "Bet";

  return (
    <div ref={barRef} className="flex flex-col gap-3 px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-3">
      {raiseOpen && (
        <div className="rounded-2xl bg-surface p-3" style={{ border: "1px solid var(--line)" }}>
          <BetSlider value={amount} bounds={bounds} onChange={setAmount} />
        </div>
      )}
      <div className="flex gap-2.5">
        <ActionButton label="Fold" hint="F" onClick={doFold} tone="danger" disabled={!!pending} />
        <ActionButton
          label={canCheck ? "Check" : `Call ${formatAmount(toCall, bounds.bb, showInBB)}`}
          hint="C"
          onClick={doCheckCall}
          tone="neutral"
          disabled={!!pending}
        />
        <ActionButton
          label={raiseOpen ? `${raiseLabel} ${formatAmount(amount, bounds.bb, showInBB)}` : raiseLabel}
          hint="R"
          onClick={doRaise}
          tone="primary"
          disabled={!!pending}
          grow
        />
      </div>
    </div>
  );
}

interface BtnProps {
  label: string;
  hint: string;
  onClick: () => void;
  tone: "danger" | "neutral" | "primary";
  disabled?: boolean;
  grow?: boolean;
}

function ActionButton({ label, hint, onClick, tone, disabled, grow }: BtnProps) {
  const bg =
    tone === "primary"
      ? "linear-gradient(180deg, var(--action-blue-hi), var(--action-blue))"
      : tone === "danger"
        ? "linear-gradient(180deg, #f07575, var(--danger))"
        : "var(--surface-3)";
  const color = tone === "neutral" ? "var(--ink)" : "#05121f";
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`no-tap-highlight relative min-h-[58px] rounded-2xl text-base font-bold tracking-tight transition-transform active:translate-y-[2px] disabled:opacity-60 ${grow ? "flex-[1.4]" : "flex-1"}`}
      style={{ background: bg, color, boxShadow: "var(--shadow-2)" }}
    >
      {label}
      <span className="absolute right-2.5 top-2 hidden text-[10px] font-medium opacity-45 sm:inline">
        {hint}
      </span>
    </button>
  );
}

export const ActionBar = memo(ActionBarImpl);
