// Full keyboard play on web: F = fold, C = check/call, R = raise,
// number keys 1-6 size the bet to a preset. Ignored when a text input is
// focused so typing a name in a modal doesn't fire a fold.

import { useEffect } from "react";

export interface KeyHandlers {
  onFold: () => void;
  onCheckCall: () => void;
  onRaise: () => void;
  onPreset: (index: number) => void;
  enabled: boolean;
}

export function useTableKeys(h: KeyHandlers): void {
  useEffect(() => {
    if (!h.enabled) return;
    function onKey(e: KeyboardEvent): void {
      const t = e.target as HTMLElement | null;
      if (
        t &&
        (t.tagName === "INPUT" ||
          t.tagName === "TEXTAREA" ||
          t.isContentEditable)
      )
        return;
      const k = e.key.toLowerCase();
      if (k === "f") {
        e.preventDefault();
        h.onFold();
      } else if (k === "c") {
        e.preventDefault();
        h.onCheckCall();
      } else if (k === "r") {
        e.preventDefault();
        h.onRaise();
      } else if (k >= "1" && k <= "6") {
        e.preventDefault();
        h.onPreset(Number(k) - 1);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [h]);
}
