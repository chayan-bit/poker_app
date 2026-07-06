// Typed wire protocol. Mirrors server/internal/protocol/messages.go EXACTLY for
// the shapes defined there (Envelope, PlaceBet, HandDealt, FairReveal, Error);
// the remaining events follow the shapes described in client/PROMPT.md. This is
// the protocol boundary: no `any` past this file.

export const PROTOCOL_VERSION = 1;

/** A card is rank char + suit char, e.g. "As", "Td", "2c". */
export type Card = string;
export type Suit = "c" | "d" | "h" | "s";

/** Every message on the wire is wrapped in this envelope. */
export interface Envelope<T = unknown> {
  v: number;
  type: string;
  /** Monotonic sequence number on server events. Absent on commands. */
  seq?: number;
  data?: T;
}

// ---- Client -> server commands (imperative) ----

export const Cmd = {
  JoinTable: "join_table",
  SitDown: "sit_down",
  PlaceBet: "place_bet",
  Leave: "leave_table",
  Resync: "resync",
  StartHand: "start_hand",
  Rebuy: "rebuy",
  SitOut: "sit_out",
  SitIn: "sit_in",
} as const;
export type CmdType = (typeof Cmd)[keyof typeof Cmd];

export type BetKind = "check" | "call" | "bet" | "raise" | "fold";

export interface JoinTableCmd {
  tableId: string;
  /** Present when opening a private table via join code. */
  joinCode?: string;
}

export interface SitDownCmd {
  tableId: string;
  seat: number;
  buyIn: number;
}

/** amount is the target to-amount for bet/raise; ignored for check/call/fold. */
export interface PlaceBetCmd {
  tableId: string;
  kind: BetKind;
  amount: number;
}

export interface LeaveCmd {
  tableId: string;
}

export interface ResyncCmd {
  tableId: string;
  /** Last seq the client successfully applied. */
  haveSeq: number;
}

/** Host-only: starts the next hand once enough seats are filled. */
export interface StartHandCmd {
  tableId: string;
}

/** Tops a sitting-out-of-chips seat back up (subject to server-side rules). */
export interface RebuyCmd {
  tableId: string;
  amount: number;
}

export interface SitOutCmd {
  tableId: string;
}

export interface SitInCmd {
  tableId: string;
}

export type Command =
  | { type: typeof Cmd.JoinTable; data: JoinTableCmd }
  | { type: typeof Cmd.SitDown; data: SitDownCmd }
  | { type: typeof Cmd.PlaceBet; data: PlaceBetCmd }
  | { type: typeof Cmd.Leave; data: LeaveCmd }
  | { type: typeof Cmd.Resync; data: ResyncCmd }
  | { type: typeof Cmd.StartHand; data: StartHandCmd }
  | { type: typeof Cmd.Rebuy; data: RebuyCmd }
  | { type: typeof Cmd.SitOut; data: SitOutCmd }
  | { type: typeof Cmd.SitIn; data: SitInCmd };

// ---- Server -> client events (past tense, render-only) ----

export const Ev = {
  HandDealt: "hand_dealt",
  BetPlaced: "bet_placed",
  StreetAdvanced: "street_advanced",
  Showdown: "showdown",
  Snapshot: "table_snapshot",
  SeatUpdate: "seat_update",
  FairReveal: "fair_reveal",
  Error: "error",
  TableStatus: "table_status",
} as const;
export type EvType = (typeof Ev)[keyof typeof Ev];

export type Street = "preflop" | "flop" | "turn" | "river" | "showdown";

/** Only YOUR hole cards are ever sent. */
export interface HandDealt {
  tableId: string;
  handId: string;
  commitment: string; // SHA-256(seed), published pre-deal
  yourSeat: number;
  yourHole: Card[];
  buttonSeat: number;
  blinds: [number, number]; // [sb, bb]
}

export interface BetPlaced {
  tableId: string;
  seat: number;
  kind: BetKind;
  /** Chips committed by this action (delta), and resulting street contribution. */
  amount: number;
  /** Resulting stack after the action. */
  stack: number;
  /** Resulting pot total. */
  pot: number;
  /** Seat now to act, or -1 if the street is closing. */
  nextToAct: number;
  /** Server deadline (epoch ms) for the next actor, for the timebank ring. */
  actByMs?: number;
  /** Total this seat has committed on the current street, if sent. */
  currentBet?: number;
  /** Chips still owed to call, if sent. */
  toCall?: number;
}

export interface StreetAdvanced {
  tableId: string;
  street: Street;
  board: Card[];
  pot: number;
  nextToAct: number;
  actByMs?: number;
}

export interface ShowdownResult {
  seat: number;
  hole: Card[];
  handClass: string; // e.g. "Full house, Kings full of Threes"
  won: number; // chips won (0 for a losing/mucked seat that is shown)
}

export interface Showdown {
  tableId: string;
  handId: string;
  board: Card[];
  results: ShowdownResult[];
  /** Pot(s) pushed; ordered for the pot-push animation. */
  pots: { amount: number; winners: number[] }[];
}

export interface SeatState {
  seat: number;
  playerId: string;
  name: string;
  stack: number;
  sittingOut: boolean;
  /** Last discrete action shown as a persistent chip-tag until next street. */
  lastAction?: { kind: BetKind; amount: number };
  /** Socket dropped, within the disconnect-grace window (server: seatView.Disconnected). */
  disconnected: boolean;
  /** Total chips this seat has committed on the current street, if sent. */
  committed?: number;
}

/** Full authoritative snapshot; applied on join and after a resync. */
export interface TableSnapshot {
  tableId: string;
  handId: string | null;
  street: Street | null;
  board: Card[];
  pot: number;
  blinds: [number, number];
  buttonSeat: number;
  maxSeats: number;
  seats: SeatState[];
  yourSeat: number | null;
  yourHole: Card[];
  nextToAct: number;
  actByMs?: number;
  /** Whether a hand is currently running (server: tableSnapshot.HandRunning). */
  handRunning: boolean;
  seq: number;
}

/** Mirrors server's seatUpdate{TableID, Seats []seatView}: a broadcast carries
 * the FULL seat list every time, not a single-seat delta. */
export interface SeatUpdate {
  tableId: string;
  seats: SeatState[];
}

/** Emitted after a hand so any client can recompute Shuffle(seed). */
export interface FairReveal {
  handId: string;
  commitment: string;
  seed: string;
}

export interface ErrorEvent {
  code: string;
  message: string;
}

/** Broadcast when the table is waiting on more seats/host action, or that
 * condition changes (e.g. seat count crosses the min-to-start threshold). */
export interface TableStatus {
  tableId: string;
  waitingForHost: boolean;
  seatedCount: number;
}

export type ServerEvent =
  | { type: typeof Ev.HandDealt; seq: number; data: HandDealt }
  | { type: typeof Ev.BetPlaced; seq: number; data: BetPlaced }
  | { type: typeof Ev.StreetAdvanced; seq: number; data: StreetAdvanced }
  | { type: typeof Ev.Showdown; seq: number; data: Showdown }
  | { type: typeof Ev.Snapshot; seq: number; data: TableSnapshot }
  | { type: typeof Ev.SeatUpdate; seq: number; data: SeatUpdate }
  | { type: typeof Ev.FairReveal; seq: number; data: FairReveal }
  | { type: typeof Ev.Error; seq?: number; data: ErrorEvent }
  | { type: typeof Ev.TableStatus; seq: number; data: TableStatus };
