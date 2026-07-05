// Settings: theme (dark default / light), four-color deck, sound, reduced
// motion, bet-preset config, BB display. Changes persist and apply immediately.

import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card, Toggle } from "@/components/ui/kit";
import {
  applyDocumentClasses,
  DEFAULT_PRESETS,
  useSettings,
  type Preset,
} from "@/store/settingsStore";

const PRESET_LABEL: Record<Preset, string> = {
  min: "Min",
  third: "⅓ pot",
  half: "½ pot",
  twothird: "⅔ pot",
  pot: "Pot",
  allin: "All-in",
};

export default function Settings() {
  const s = useSettings();

  useEffect(() => {
    applyDocumentClasses(s.theme, s.reducedMotion);
  }, [s.theme, s.reducedMotion]);

  const togglePreset = (p: Preset) => {
    const has = s.presets.includes(p);
    const next = has
      ? s.presets.filter((x) => x !== p)
      : DEFAULT_PRESETS.filter((x) => x === p || s.presets.includes(x));
    s.setPresets(next.length ? next : [p]);
  };

  return (
    <Screen title="Settings" back={<Back />}>
      <Card>
        <div className="flex items-center justify-between py-2">
          <span className="text-base">Theme</span>
          <div className="flex overflow-hidden rounded-lg border border-line">
            {(["dark", "light"] as const).map((t) => (
              <button
                key={t}
                onClick={() => s.setTheme(t)}
                className="px-4 py-1.5 text-sm capitalize"
                style={{
                  background: s.theme === t ? "var(--action-blue)" : "transparent",
                  color: s.theme === t ? "#04121f" : "var(--ink-dim)",
                }}
              >
                {t}
              </button>
            ))}
          </div>
        </div>
        <Toggle label="Four-color deck" checked={s.fourColorDeck} onChange={s.toggleFourColor} />
        <Toggle label="Sound" checked={s.sound} onChange={s.toggleSound} />
        <Toggle label="Reduced motion" checked={s.reducedMotion} onChange={s.toggleReducedMotion} />
        <Toggle label="Show amounts in big blinds" checked={s.showInBB} onChange={s.toggleShowInBB} />
      </Card>

      <Card>
        <p className="mb-3 text-sm font-semibold uppercase tracking-wide text-ink-dim">
          Bet presets
        </p>
        <div className="flex flex-wrap gap-2">
          {DEFAULT_PRESETS.map((p) => {
            const on = s.presets.includes(p);
            return (
              <button
                key={p}
                onClick={() => togglePreset(p)}
                className="rounded-full px-3 py-1.5 text-sm"
                style={{
                  background: on ? "var(--action-blue)" : "var(--surface-3)",
                  color: on ? "#04121f" : "var(--ink)",
                }}
              >
                {PRESET_LABEL[p]}
              </button>
            );
          })}
        </div>
      </Card>
    </Screen>
  );
}

function Back() {
  const nav = useNavigate();
  return (
    <button
      onClick={() => nav(-1)}
      className="grid h-9 w-9 place-items-center rounded-lg border border-line text-ink-dim"
      aria-label="Back"
    >
      ←
    </button>
  );
}
