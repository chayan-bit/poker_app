// Reactive UI state for the nearby (offline P2P) flow. Kept separate from the
// game store: gameStore renders the table from confirmed events exactly as it
// does online, while THIS store holds only nearby-specific chrome - which screen
// is showing, the session-scoped settings, presence/void toasts, a detected
// dishonest-dealer flag, and the end-of-session summary. The NearbySession
// controller is the only writer via these setters.

import { create } from "zustand";

export type NearbyPhase = "setup" | "host" | "join" | "table" | "summary";

/** One player's line in the end-of-session summary. */
export interface SummaryRow {
  playerId: string;
  name: string;
  buyIn: number;
  finalStack: number;
  net: number;
}

export interface SessionSummary {
  handsPlayed: number;
  biggestPot: number;
  rows: SummaryRow[];
}

/** Host-chosen, session-scoped ruleset. These chips never touch cloud balances. */
export interface NearbyConfig {
  tableName: string;
  smallBlind: number;
  bigBlind: number;
  startingStack: number;
}

interface NearbyState {
  phase: NearbyPhase;
  config: NearbyConfig;
  /** Subtle "shuffling together" micro-state during the fair commit/reveal round. */
  shuffling: boolean;
  /** "Hand voided - <name> was dealing" toast; cleared by the UI. */
  voidToast: string | null;
  /** Names of peers currently within the disconnect-grace window. */
  reconnecting: string[];
  /** Hand id flagged by the mesh as dealt with a dishonest seed, or null. */
  dishonestHand: string | null;
  summary: SessionSummary | null;

  setPhase: (phase: NearbyPhase) => void;
  setConfig: (config: NearbyConfig) => void;
  setShuffling: (on: boolean) => void;
  setVoidToast: (msg: string | null) => void;
  setReconnecting: (names: string[]) => void;
  setDishonest: (handId: string | null) => void;
  setSummary: (summary: SessionSummary | null) => void;
  reset: () => void;
}

const DEFAULT_CONFIG: NearbyConfig = {
  tableName: "Kitchen table",
  smallBlind: 1,
  bigBlind: 2,
  startingStack: 200,
};

export const useNearby = create<NearbyState>((set) => ({
  phase: "setup",
  config: { ...DEFAULT_CONFIG },
  shuffling: false,
  voidToast: null,
  reconnecting: [],
  dishonestHand: null,
  summary: null,

  setPhase: (phase) => set({ phase }),
  setConfig: (config) => set({ config }),
  setShuffling: (shuffling) => set({ shuffling }),
  setVoidToast: (voidToast) => set({ voidToast }),
  setReconnecting: (reconnecting) => set({ reconnecting }),
  setDishonest: (dishonestHand) => set({ dishonestHand }),
  setSummary: (summary) => set({ summary }),
  reset: () =>
    set({
      phase: "setup",
      shuffling: false,
      voidToast: null,
      reconnecting: [],
      dishonestHand: null,
      summary: null,
    }),
}));
