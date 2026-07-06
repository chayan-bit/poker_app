// Adapts a MeshNode's per-recipient event envelopes into the exact ServerEvent
// dispatch path the WebSocket client feeds the game store, so Table, ActionBar,
// showdown, and the fairness verifier render an offline hand identically to an
// online one (issue #29).
//
// Two shape gaps are reconciled here, at the protocol boundary:
//   1. The WASM localcore emits table-package wire shapes (toAct, map-keyed
//      showdown results, engine Awards) while the client store consumes the
//      protocol.ts shapes the mock/WS path uses (nextToAct, ShowdownResult[],
//      pots). This module translates the former into the latter.
//   2. localcore bet_placed carries no post-action stack. We recover each seat's
//      absolute stack from pot deltas: the pot only ever grows by exactly the
//      chips the acting seat just committed, so stack = handStartStack minus the
//      running committed total. Blinds (the one non-action commit) are posted
//      into the model at deal time. Authoritative seat_update stacks reset the
//      baseline every hand, so any approximation self-heals within one hand.
//
// The bridge NEVER runs game logic; it only reshapes confirmed events and routes
// the local player's commands to the mesh coordinator.

import { BROADCAST, type Envelope, type EventMap, type LocalConfig } from "./core";
import type { NetHandlers, NetTransport } from "@/net/client";
import {
  Cmd,
  Ev,
  type Card,
  type Command,
  type SeatState,
  type ServerEvent,
  type ShowdownResult,
  type Street,
  type TableSnapshot,
} from "@/net/protocol";

// ---- localcore raw event bodies (table-package wire shapes) ----

interface RawSeat {
  seat: number;
  playerId: string;
  stack: number;
  sittingOut: boolean;
  inHand: boolean;
  committed?: number;
  disconnected: boolean;
}
interface RawBet {
  seat: number;
  kind: string;
  amount: number;
  pot: number;
  toAct: number;
  currentBet: number;
  toCall: number;
}
interface RawStreet {
  street: Street;
  board: Card[];
}
interface RawAward {
  SeatID: number;
  Amount: number;
}
interface RawShowdown {
  handId: string;
  board: Card[];
  results: Record<string, string>;
  awards: RawAward[];
  revealed: Record<string, Card[]>;
}

/** Wiring the bridge needs from its owning session. */
export interface BridgeDeps {
  selfId: string;
  config: LocalConfig;
  /** Resolves a peer id to the human name to render on the felt. */
  nameFor: (playerId: string) => string;
  /** Routes a local command envelope to the mesh coordinator. */
  submit: (envelope: object) => void;
  /** Fired when a hand is voided by the mesh (dealer dropped mid-hand). */
  onVoid?: () => void;
}

const SB = 0;
const BB = 1;

export class MeshBridge {
  private handlers: NetHandlers | null = null;
  private seq = 0;

  // Absolute stack per seat at the start of the current hand, and the chips
  // committed since; live stack = base minus committed.
  private stackBase = new Map<number, number>();
  private committed = new Map<number, number>();
  private pot = 0;
  private lastSeats: RawSeat[] = [];

  constructor(private readonly deps: BridgeDeps) {}

  /** The store's connectLocal builder: wires the sink and returns the transport. */
  build = (handlers: NetHandlers): NetTransport => {
    this.handlers = handlers;
    handlers.onStatus("open");
    this.emit(Ev.Snapshot, this.initialSnapshot());
    return {
      send: (cmd: Command) => this.route(cmd),
      close: () => {
        this.handlers = null;
      },
    };
  };

  /** MeshNode `onApplied` hook: reshape one applied entry's events for the store. */
  onApplied = (entry: { kind: string }, events: EventMap): void => {
    if (entry.kind === "void") {
      this.deps.onVoid?.();
    }
    if (!this.handlers) return;
    for (const key of [BROADCAST, this.deps.selfId]) {
      for (const env of events[key] ?? []) this.translate(env);
    }
  };

  private route(cmd: Command): void {
    switch (cmd.type) {
      case Cmd.PlaceBet:
        this.deps.submit({ v: 1, type: "place_bet", data: { kind: cmd.data.kind, amount: cmd.data.amount } });
        break;
      case Cmd.SitOut:
        this.deps.submit({ v: 1, type: "sit_out", data: { tableId: this.deps.config.id } });
        break;
      case Cmd.SitIn:
        this.deps.submit({ v: 1, type: "sit_in", data: { tableId: this.deps.config.id } });
        break;
      case Cmd.Rebuy:
        this.deps.submit({ v: 1, type: "rebuy", data: { tableId: this.deps.config.id, amount: cmd.data.amount } });
        break;
      // join_table / resync / start_hand / leave_table have no mesh equivalent:
      // the mesh auto-sequences hands and heals via gossip. Safely ignored.
      default:
        break;
    }
  }

  private translate(env: Envelope): void {
    const data = env.data as unknown;
    switch (env.type) {
      case Ev.SeatUpdate:
        this.onSeatUpdate(data as { seats: RawSeat[] });
        break;
      case Ev.HandDealt:
        this.onHandDealt(data as { buttonSeat: number; blinds: [number, number] });
        this.emit(Ev.HandDealt, data);
        break;
      case Ev.BetPlaced:
        this.emit(Ev.BetPlaced, this.onBet(data as RawBet));
        break;
      case Ev.StreetAdvanced:
        this.emit(Ev.StreetAdvanced, this.onStreet(data as RawStreet));
        break;
      case Ev.Showdown:
        this.emit(Ev.Showdown, this.onShowdown(data as RawShowdown));
        break;
      case Ev.FairReveal:
      case Ev.TableStatus:
      case Ev.Error:
        this.emit(env.type, data);
        break;
      default:
        break;
    }
  }

  private onSeatUpdate(d: { seats: RawSeat[] }): void {
    this.lastSeats = d.seats;
    for (const s of d.seats) {
      this.stackBase.set(s.seat, s.stack);
      this.committed.set(s.seat, 0);
    }
    this.pot = 0;
    const seats: SeatState[] = d.seats.map((s) => ({
      seat: s.seat,
      playerId: s.playerId,
      name: this.deps.nameFor(s.playerId),
      stack: s.stack,
      sittingOut: s.sittingOut,
      disconnected: s.disconnected,
      committed: s.committed ?? 0,
    }));
    this.emit(Ev.SeatUpdate, { tableId: this.deps.config.id, seats });
  }

  private onHandDealt(d: { buttonSeat: number; blinds: [number, number] }): void {
    // New hand: reset committed and post blinds into the stack model so the
    // first voluntary action's pot delta is attributed to the right seat.
    for (const seat of this.stackBase.keys()) this.committed.set(seat, 0);
    this.pot = 0;
    const inHand = this.lastSeats
      .filter((s) => !s.sittingOut && s.stack > 0)
      .map((s) => s.seat)
      .sort((a, b) => a - b);
    if (inHand.length < 2) return;
    const btn = Math.max(0, inHand.indexOf(d.buttonSeat));
    const heads = inHand.length === 2;
    const sbSeat = heads ? inHand[btn] : inHand[(btn + 1) % inHand.length];
    const bbSeat = heads ? inHand[(btn + 1) % 2] : inHand[(btn + 2) % inHand.length];
    this.postBlind(sbSeat, d.blinds[SB]);
    this.postBlind(bbSeat, d.blinds[BB]);
  }

  private postBlind(seat: number, blind: number): void {
    const base = this.stackBase.get(seat) ?? 0;
    const put = Math.min(blind, base);
    this.committed.set(seat, (this.committed.get(seat) ?? 0) + put);
    this.pot += put;
  }

  private liveStack(seat: number): number {
    return (this.stackBase.get(seat) ?? 0) - (this.committed.get(seat) ?? 0);
  }

  private onBet(d: RawBet): {
    tableId: string;
    seat: number;
    kind: string;
    amount: number;
    stack: number;
    pot: number;
    nextToAct: number;
    currentBet: number;
    toCall: number;
  } {
    const delta = Math.max(0, d.pot - this.pot);
    this.committed.set(d.seat, (this.committed.get(d.seat) ?? 0) + delta);
    this.pot = d.pot;
    return {
      tableId: this.deps.config.id,
      seat: d.seat,
      kind: d.kind,
      amount: d.amount,
      stack: this.liveStack(d.seat),
      pot: d.pot,
      nextToAct: d.toAct,
      currentBet: d.currentBet,
      toCall: d.toCall,
    };
  }

  private onStreet(d: RawStreet): {
    tableId: string;
    street: Street;
    board: Card[];
    pot: number;
    nextToAct: number;
  } {
    return { tableId: this.deps.config.id, street: d.street, board: d.board, pot: this.pot, nextToAct: -1 };
  }

  private onShowdown(d: RawShowdown): {
    tableId: string;
    handId: string;
    board: Card[];
    results: ShowdownResult[];
    pots: { amount: number; winners: number[] }[];
  } {
    const wonBySeat = new Map<number, number>();
    for (const a of d.awards ?? []) wonBySeat.set(a.SeatID, (wonBySeat.get(a.SeatID) ?? 0) + a.Amount);
    const seatIds = new Set<number>([
      ...Object.keys(d.revealed ?? {}).map(Number),
      ...(d.awards ?? []).map((a) => a.SeatID),
    ]);
    const results: ShowdownResult[] = [...seatIds].map((seat) => ({
      seat,
      hole: d.revealed?.[seat] ?? [],
      handClass: d.results?.[seat] ?? "",
      won: wonBySeat.get(seat) ?? 0,
    }));
    const pots = (d.awards ?? []).map((a) => ({ amount: a.Amount, winners: [a.SeatID] }));
    // Keep the model's baseline in step so between-hand stacks include winnings.
    for (const [seat, won] of wonBySeat) this.stackBase.set(seat, this.liveStack(seat) + won);
    return { tableId: this.deps.config.id, handId: d.handId, board: d.board, results, pots };
  }

  private initialSnapshot(): TableSnapshot {
    return {
      tableId: this.deps.config.id,
      handId: null,
      street: null,
      board: [],
      pot: 0,
      blinds: [this.deps.config.smallBlind, this.deps.config.bigBlind],
      buttonSeat: -1,
      maxSeats: this.deps.config.maxSeats ?? 9,
      seats: [],
      yourSeat: null,
      yourHole: [],
      nextToAct: -1,
      handRunning: false,
      seq: 0,
    };
  }

  private emit(type: string, data: unknown): void {
    this.handlers?.onEvent({ type, seq: ++this.seq, data } as ServerEvent);
  }
}
