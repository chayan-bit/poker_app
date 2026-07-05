// Bet sizing: a horizontal slider PLUS configurable preset pills
// (min / 1/3 / 1/2 / 2/3 / pot / all-in). Presets compute a to-amount from the
// pot and betting bounds. The chosen amount is the raise/bet target.

import { memo } from "react";
import type { Preset } from "@/store/settingsStore";
import { useSettings } from "@/store/settingsStore";
import { formatAmount } from "@/lib/format";

export interface BetBounds {
  min: number; // min legal to-amount
  max: number; // all-in to-amount
  pot: number;
  bb: number;
}

interface Props {
  value: number;
  bounds: BetBounds;
  onChange: (v: number) => void;
}

const LABEL: Record<Preset, string> = {
  min: "Min",
  third: "⅓",
  half: "½",
  twothird: "⅔",
  pot: "Pot",
  allin: "All-in",
};

export function presetAmount(preset: Preset, b: BetBounds): number {
  const raw =
    preset === "min"
      ? b.min
      : preset === "allin"
        ? b.max
        : preset === "pot"
          ? b.pot
          : preset === "third"
            ? Math.round(b.pot / 3)
            : preset === "half"
              ? Math.round(b.pot / 2)
              : Math.round((b.pot * 2) / 3);
  return Math.max(b.min, Math.min(b.max, raw));
}

function BetSliderImpl({ value, bounds, onChange }: Props) {
  const presets = useSettings((s) => s.presets);
  const showInBB = useSettings((s) => s.showInBB);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-1.5">
        {presets.map((p) => {
          const amt = presetAmount(p, bounds);
          const active = amt === value;
          return (
            <button
              key={p}
              onClick={() => onChange(amt)}
              className="num min-h-9 rounded-full px-3 py-1 text-sm font-medium transition-colors"
              style={{
                background: active ? "var(--action-blue)" : "var(--surface-3)",
                color: active ? "#04121f" : "var(--ink)",
              }}
            >
              {LABEL[p]}
            </button>
          );
        })}
      </div>
      <div className="flex items-center gap-3">
        <input
          type="range"
          min={bounds.min}
          max={bounds.max}
          step={bounds.bb}
          value={value}
          onChange={(e) => onChange(Number(e.target.value))}
          className="h-2 flex-1 cursor-pointer appearance-none rounded-full"
          style={{
            accentColor: "var(--action-blue)",
            background: "var(--surface-3)",
          }}
          aria-label="Bet amount"
        />
        <span
          className="num w-24 text-right text-base font-semibold"
          style={{ color: "var(--action-blue)" }}
        >
          {formatAmount(value, bounds.bb, showInBB)}
        </span>
      </div>
    </div>
  );
}

export const BetSlider = memo(BetSliderImpl);
