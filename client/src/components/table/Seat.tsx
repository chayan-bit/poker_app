// A player's seat. Persistent DOM node positioned by the parent via a transform
// (it MOVES, it does not remount). Gradient avatar + monogram, name, stack
// (tabular) on a frosted plaque, the timebank ring when active, and a
// persistent last-action chip-tag ("raise 240") that stays until the next
// street. The winning seat gets a gold glow.

import { memo } from "react";
import type { BetKind, SeatState } from "@/net/protocol";
import { formatAmount } from "@/lib/format";
import { useSettings } from "@/store/settingsStore";
import { avatarGradient, initials } from "@/lib/avatar";
import { TimerRing } from "./TimerRing";
import { Card } from "./Card";
import { ChipStack } from "./Chips";
import { useTimebank } from "@/hooks/useTimebank";

interface Props {
  player: SeatState;
  x: number;
  y: number;
  isHero: boolean;
  isButton: boolean;
  isActive: boolean;
  folded: boolean;
  winner: boolean;
  bb: number;
  heroHole: string[];
  actByMs: number | null;
}

const AVATAR = 58;

function actionText(kind: BetKind, amount: number, bb: number, inBB: boolean): string {
  if (kind === "fold") return "Fold";
  if (kind === "check") return "Check";
  if (kind === "call") return `Call ${formatAmount(amount, bb, inBB)}`;
  return `${kind === "raise" ? "Raise" : "Bet"} ${formatAmount(amount, bb, inBB)}`;
}

function SeatImpl(p: Props) {
  const showInBB = useSettings((s) => s.showInBB);
  const tb = useTimebank(p.isActive ? p.actByMs : null);
  const { player } = p;
  const bet = player.lastAction && player.lastAction.kind !== "fold" && player.lastAction.kind !== "check";

  return (
    <div
      className="gpu absolute left-0 top-0 flex w-32 flex-col items-center"
      style={{
        transform: `translate(${p.x}px, ${p.y}px) translate(-50%, -50%)`,
        transition: "transform var(--dur-slow) var(--ease)",
        opacity: p.folded ? 0.5 : 1,
      }}
      data-seat={player.seat}
    >
      {/* Bet chips sit between the seat and the pot (toward center). */}
      {bet && player.lastAction && (
        <div className="mb-1 flex items-center gap-1">
          <ChipStack amount={player.lastAction.amount} size={20} maxChips={4} />
          <span
            className="num rounded-full px-1.5 text-[11px] font-semibold"
            style={{ color: "var(--gold)" }}
          >
            {formatAmount(player.lastAction.amount, p.bb, showInBB)}
          </span>
        </div>
      )}

      {p.isHero && p.heroHole.length > 0 && (
        <div className="mb-1 flex gap-1">
          {p.heroHole.map((c, i) => (
            <Card key={i} card={c} size="md" dimmed={p.folded} />
          ))}
        </div>
      )}

      <div className="relative grid place-items-center" style={{ width: AVATAR, height: AVATAR }}>
        {p.winner && (
          <span className="absolute inset-[-5px] rounded-full" style={{ boxShadow: "var(--glow-gold)" }} />
        )}
        <TimerRing active={p.isActive} fraction={tb.fraction} urgent={tb.urgent} size={AVATAR + 10} />
        <div
          className="grid h-full w-full place-items-center rounded-full text-lg font-bold text-white"
          style={{
            background: avatarGradient(player.name || String(player.seat)),
            boxShadow: p.isActive ? "var(--glow-blue)" : "var(--shadow-2)",
            border: "2px solid rgba(255,255,255,0.12)",
          }}
        >
          {p.isHero ? "★" : initials(player.name)}
        </div>
        {p.isButton && (
          <span
            className="absolute -bottom-1 -right-1 grid h-6 w-6 place-items-center rounded-full text-[11px] font-black text-black"
            style={{ background: "var(--gold)", boxShadow: "var(--shadow-1)" }}
          >
            D
          </span>
        )}
      </div>

      {/* name + stack plaque */}
      <div
        className="mt-1.5 flex min-w-[92px] flex-col items-center rounded-lg px-2 py-1"
        style={{ background: "rgba(10,14,18,0.6)", border: "1px solid var(--line-hi)" }}
      >
        <div className="max-w-[88px] truncate text-xs font-medium text-ink">
          {p.isHero ? "You" : player.name}
        </div>
        <div className="num text-sm font-semibold" style={{ color: p.isActive ? "var(--action-blue-hi)" : "var(--ink-dim)" }}>
          {formatAmount(player.stack, p.bb, showInBB)}
        </div>
      </div>

      {/* fold/check chip-tag (non-bet actions) */}
      {player.lastAction && !bet && (
        <div
          className="num mt-1 rounded-full px-2 py-0.5 text-[11px] font-medium"
          style={{
            background: "rgba(0,0,0,0.45)",
            color: player.lastAction.kind === "fold" ? "var(--danger)" : "var(--ink-dim)",
          }}
        >
          {actionText(player.lastAction.kind, player.lastAction.amount, p.bb, showInBB)}
        </div>
      )}
    </div>
  );
}

export const Seat = memo(SeatImpl);
