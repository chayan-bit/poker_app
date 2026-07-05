// User settings, persisted to localStorage. These drive theme, deck, motion,
// sound, bet presets, and BB display. Applying theme/motion classes to <html>
// is a side effect run from applyDocumentClasses().

import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Theme = "dark" | "light";

/** Bet presets are fractions of the pot; "allin" is a sentinel. */
export type Preset = "min" | "third" | "half" | "twothird" | "pot" | "allin";

export const DEFAULT_PRESETS: Preset[] = [
  "min",
  "third",
  "half",
  "twothird",
  "pot",
  "allin",
];

interface SettingsState {
  theme: Theme;
  fourColorDeck: boolean;
  sound: boolean;
  reducedMotion: boolean;
  showInBB: boolean;
  presets: Preset[];
  setTheme: (t: Theme) => void;
  toggleFourColor: () => void;
  toggleSound: () => void;
  toggleReducedMotion: () => void;
  toggleShowInBB: () => void;
  setPresets: (p: Preset[]) => void;
}

export const useSettings = create<SettingsState>()(
  persist(
    (set) => ({
      theme: "dark",
      fourColorDeck: false,
      sound: true,
      reducedMotion: false,
      showInBB: false,
      presets: DEFAULT_PRESETS,
      setTheme: (theme) => set({ theme }),
      toggleFourColor: () =>
        set((s) => ({ fourColorDeck: !s.fourColorDeck })),
      toggleSound: () => set((s) => ({ sound: !s.sound })),
      toggleReducedMotion: () =>
        set((s) => ({ reducedMotion: !s.reducedMotion })),
      toggleShowInBB: () => set((s) => ({ showInBB: !s.showInBB })),
      setPresets: (presets) => set({ presets }),
    }),
    { name: "poker:settings" },
  ),
);

/** Sync theme + reduced-motion to <html> classes. Call on change. */
export function applyDocumentClasses(theme: Theme, reduced: boolean): void {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.classList.toggle("light", theme === "light");
  root.classList.toggle("reduce-motion", reduced);
}
