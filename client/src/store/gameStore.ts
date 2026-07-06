// The single render-side store. It NEVER computes game outcomes: it applies
// server-confirmed events and, for the local player only, previews the local
// action optimistically (tagged pending) until the server confirms or the
// preview is rolled back.
//
// Re-render granularity is per discrete event (an event handler mutates state
// once), never per frame. Components subscribe to slices so the whole table
// doesn't re-render on every field change.

import { create } from "zustand";
import { WsClient, type ConnStatus, type NetTransport } from "@/net/client";
import { MockServer } from "@/net/mockServer";
import { verifyCommitment } from "@/lib/sha";
import {
  Cmd,
  Ev,
  type BetKind,
  type Card,
  type SeatState,
  type ServerEvent,
  type Street,
  type TableSnapshot,
  type TourneyResult,
} from "@/net/protocol";

/** How long an optimistic action may wait for a confirming bet_placed event
 * before it's treated as lost and the client falls back to a resync. */
const PENDING_ACTION_TIMEOUT_MS = 3000;

export interface FairnessRecord {
  handId: string;
  commitment: string;
  seed: string | null;
  verified: true | false | "pending";
}

const MAX_FAIRNESS_RECORDS = 50;

/** A transient "blinds up" banner: cleared by the UI after its display window
 * (or by clearBlindsUpBanner). `at` lets the banner component key its
 * fade-in even if the same level somehow repeats. */
export interface BlindsUpBanner {
  level: number;
  sb: number;
  bb: number;
  at: number;
}

/** The most recent elimination, kept until the UI dismisses it. Includes the
 * busted seat's last-known name so the toast/placement screen can render it
 * even after seat_update removes the seat from the table. */
export interface EliminationEvent {
  seat: number;
  playerId: string;
  place: number;
  name: string;
  at: number;
}

export interface TourneyState {
  level: number;
  sb: number;
  bb: number;
  /** This player's final finishing place, set once they are eliminated (or
   * they win, in which case tourneyResult carries place 1). */
  myPlace: number | null;
  blindsUp: BlindsUpBanner | null;
  lastElimination: EliminationEvent | null;
  result: TourneyResult | null;
}

const EMPTY_TOURNEY: TourneyState = {
  level: 0,
  sb: 0,
  bb: 0,
  myPlace: null,
  blindsUp: null,
  lastElimination: null,
  result: null,
};

export interface PendingAction {
  kind: BetKind;
  amount: number;
  /** local id so a confirm can clear the right preview */
  at: number;
}

export interface HandRecord {
  handId: string;
  board: Card[];
  events: ServerEvent[];
  commitment?: string;
  seed?: string;
}

interface GameState {
  // ---- connection ----
  status: ConnStatus;
  transport: NetTransport | null;
  usingMock: boolean;

  // ---- table (the stable scene graph's data) ----
  tableId: string | null;
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
  actByMs: number | null;
  handRunning: boolean;

  // ---- optimistic layer ----
  pending: PendingAction | null;
  rollbackNonce: number; // bump to trigger the action-bar shake

  // ---- ancillary ----
  lastError: { code: string; message: string } | null;
  history: HandRecord[];
  reveals: Record<string, { commitment: string; seed: string }>;

  // ---- fairness auto-verification ----
  fairness: FairnessRecord[];

  // ---- table status (waiting-for-host / seated-count banner) ----
  tableStatus: { waitingForHost: boolean; seatedCount: number } | null;

  // ---- rebuy sheet state ----
  rebuyPending: boolean;
  rebuyError: string | null;

  // ---- tournament (sit-and-go) slice ----
  tourney: TourneyState;

  // ---- actions ----
  connect: (opts: { url?: string; mock?: boolean; token?: string }) => void;
  disconnect: () => void;
  act: (kind: BetKind, amount: number) => void;
  fold: () => void;
  check: () => void;
  call: () => void;
  bet: (amount: number) => void;
  raise: (amount: number) => void;
  clearError: () => void;
  startHand: () => void;
  rebuy: (amount: number) => void;
  clearRebuyError: () => void;
  sitOut: () => void;
  sitIn: () => void;

  // ---- tourney actions ----
  clearBlindsUpBanner: () => void;
  clearElimination: () => void;
  clearTourneyResult: () => void;

  // ---- fairness selectors ----
  getFairnessRecord: (handId: string) => FairnessRecord | undefined;
  getFairnessRecords: () => FairnessRecord[];
}

const EMPTY_TABLE = {
  tableId: null,
  handId: null,
  street: null as Street | null,
  board: [] as Card[],
  pot: 0,
  blinds: [0, 0] as [number, number],
  buttonSeat: -1,
  maxSeats: 9,
  seats: [] as SeatState[],
  yourSeat: null as number | null,
  yourHole: [] as Card[],
  nextToAct: -1,
  actByMs: null as number | null,
  handRunning: false,
};

export const useGame = create<GameState>((set, get) => {
  function applyEvent(ev: ServerEvent): void {
    recordForReplay(ev);
    switch (ev.type) {
      case Ev.Snapshot:
        applySnapshot(ev.data);
        break;
      case Ev.HandDealt: {
        const d = ev.data;
        set({
          handId: d.handId,
          buttonSeat: d.buttonSeat,
          blinds: d.blinds,
          yourSeat: d.yourSeat,
          yourHole: d.yourHole,
          street: "preflop",
          board: [],
          pending: null,
          handRunning: true,
        });
        break;
      }
      case Ev.BetPlaced: {
        const d = ev.data;
        set((s) => {
          const seats = s.seats.map((seat) =>
            seat.seat === d.seat
              ? {
                  ...seat,
                  stack: d.stack,
                  lastAction: { kind: d.kind, amount: d.amount },
                }
              : seat,
          );
          // The server confirmed the local player's action -> clear pending.
          const clearPending = d.seat === s.yourSeat;
          return {
            seats,
            pot: d.pot,
            nextToAct: d.nextToAct,
            actByMs: d.actByMs ?? null,
            pending: clearPending ? null : s.pending,
          };
        });
        break;
      }
      case Ev.StreetAdvanced: {
        const d = ev.data;
        set((s) => ({
          street: d.street,
          board: d.board,
          pot: d.pot,
          nextToAct: d.nextToAct,
          actByMs: d.actByMs ?? null,
          seats: s.seats.map((seat) => ({ ...seat, lastAction: undefined })),
        }));
        break;
      }
      case Ev.Showdown: {
        const d = ev.data;
        set((s) => {
          const seats = s.seats.map((seat) => {
            const r = d.results.find((x) => x.seat === seat.seat);
            return r ? { ...seat, stack: seat.stack + r.won } : seat;
          });
          return {
            seats,
            board: d.board,
            street: "showdown",
            nextToAct: -1,
            handRunning: false,
          };
        });
        break;
      }
      case Ev.SeatUpdate: {
        const d = ev.data;
        // Server broadcasts the FULL seat list every time (not a per-seat
        // delta), so the local list is simply replaced, sorted by seat.
        set((s) => {
          const seats = [...d.seats].sort((a, b) => a.seat - b.seat);
          const rebuySettled = s.rebuyPending;
          return {
            seats,
            rebuyPending: rebuySettled ? false : s.rebuyPending,
            rebuyError: rebuySettled ? null : s.rebuyError,
          };
        });
        break;
      }
      case Ev.FairReveal: {
        const d = ev.data;
        set((s) => ({
          reveals: {
            ...s.reveals,
            [d.handId]: { commitment: d.commitment, seed: d.seed },
          },
          fairness: [
            {
              handId: d.handId,
              commitment: d.commitment,
              seed: d.seed,
              verified: "pending" as const,
            },
            ...s.fairness,
          ].slice(0, MAX_FAIRNESS_RECORDS),
        }));
        void verifyCommitment(d.seed, d.commitment).then((result) => {
          set((s) => ({
            fairness: s.fairness.map((f) =>
              f.handId === d.handId ? { ...f, verified: result.ok } : f,
            ),
          }));
        });
        break;
      }
      case Ev.TableStatus: {
        const d = ev.data;
        set({
          tableStatus: {
            waitingForHost: d.waitingForHost,
            seatedCount: d.seatedCount,
          },
        });
        break;
      }
      case Ev.Error: {
        const d = ev.data;
        set((s) => {
          // A rebuy error is surfaced inline inside the rebuy sheet, never as
          // a rollback/shake on the felt (the sheet isn't a betting preview).
          if (s.rebuyPending) {
            return { rebuyPending: false, rebuyError: d.message };
          }
          // An error on our own pending action is a rollback: undo + shake.
          return {
            lastError: d,
            pending: null,
            rollbackNonce: s.pending ? s.rollbackNonce + 1 : s.rollbackNonce,
          };
        });
        break;
      }
      case Ev.BlindsUp: {
        const d = ev.data;
        set((s) => ({
          tourney: {
            ...s.tourney,
            level: d.level,
            sb: d.sb,
            bb: d.bb,
            blindsUp: { level: d.level, sb: d.sb, bb: d.bb, at: Date.now() },
          },
          // The header blinds label reads off the top-level `blinds` tuple;
          // update it immediately rather than waiting for the next hand_dealt.
          blinds: [d.sb, d.bb],
        }));
        break;
      }
      case Ev.Elimination: {
        const d = ev.data;
        set((s) => {
          const bustedSeat = s.seats.find((seat) => seat.seat === d.seat);
          const isHero = d.seat === s.yourSeat;
          return {
            tourney: {
              ...s.tourney,
              myPlace: isHero ? d.place : s.tourney.myPlace,
              lastElimination: {
                seat: d.seat,
                playerId: d.playerId,
                place: d.place,
                name: bustedSeat?.name ?? d.playerId,
                at: Date.now(),
              },
            },
          };
        });
        break;
      }
      case Ev.TourneyResult: {
        const d = ev.data;
        set((s) => ({ tourney: { ...s.tourney, result: d } }));
        break;
      }
    }
  }

  function applySnapshot(d: TableSnapshot): void {
    set({
      tableId: d.tableId,
      handId: d.handId,
      street: d.street,
      board: d.board,
      pot: d.pot,
      blinds: d.blinds,
      buttonSeat: d.buttonSeat,
      maxSeats: d.maxSeats,
      seats: d.seats,
      yourSeat: d.yourSeat,
      yourHole: d.yourHole,
      nextToAct: d.nextToAct,
      actByMs: d.actByMs ?? null,
      handRunning: d.handRunning,
      // A snapshot is ground truth: any optimistic preview is superseded.
      pending: null,
    });
  }

  function recordForReplay(ev: ServerEvent): void {
    if (ev.type === Ev.HandDealt) {
      set((s) => ({
        history: [
          {
            handId: ev.data.handId,
            board: [],
            events: [ev],
            commitment: ev.data.commitment,
          },
          ...s.history,
        ].slice(0, 50),
      }));
      return;
    }
    set((s) => {
      if (s.history.length === 0) return {};
      const [cur, ...rest] = s.history;
      const next: HandRecord = { ...cur, events: [...cur.events, ev] };
      if (ev.type === Ev.Showdown) next.board = ev.data.board;
      if (ev.type === Ev.FairReveal) next.seed = ev.data.seed;
      return { history: [next, ...rest] };
    });
  }

  // Guards the single in-flight optimistic action: if no confirming
  // bet_placed / error arrives within PENDING_ACTION_TIMEOUT_MS, clear the
  // preview and fall back to a resync rather than leave the UI stuck.
  let pendingTimer: number | null = null;
  function clearPendingTimer(): void {
    if (pendingTimer !== null) {
      window.clearTimeout(pendingTimer);
      pendingTimer = null;
    }
  }

  const handlers = {
    onEvent: (ev: ServerEvent) => {
      if (
        (ev.type === Ev.BetPlaced && ev.data.seat === get().yourSeat) ||
        ev.type === Ev.Error ||
        ev.type === Ev.Snapshot
      ) {
        clearPendingTimer();
      }
      applyEvent(ev);
    },
    onStatus: (status: ConnStatus) => set({ status }),
    onGap: (lastGood: number) => {
      const { transport, tableId } = get();
      if (transport && tableId) {
        transport.send({
          type: Cmd.Resync,
          data: { tableId, haveSeq: lastGood },
        });
      }
    },
  };

  return {
    status: "closed",
    transport: null,
    usingMock: false,
    ...EMPTY_TABLE,
    pending: null,
    rollbackNonce: 0,
    lastError: null,
    history: [],
    reveals: {},
    fairness: [],
    tableStatus: null,
    rebuyPending: false,
    rebuyError: null,
    tourney: { ...EMPTY_TOURNEY },

    connect: ({ url, mock, token }) => {
      get().transport?.close();
      clearPendingTimer();
      if (mock || !url) {
        const server = new MockServer(handlers);
        set({ transport: server, usingMock: true });
        server.connect();
      } else {
        const client = new WsClient(url, handlers, {
          token,
          tableId: get().tableId ?? undefined,
        });
        set({ transport: client, usingMock: false });
        client.connect();
      }
    },

    disconnect: () => {
      get().transport?.close();
      clearPendingTimer();
      set({
        transport: null,
        status: "closed",
        ...EMPTY_TABLE,
        rebuyPending: false,
        rebuyError: null,
        tourney: { ...EMPTY_TOURNEY },
      });
    },

    act: (kind, amount) => {
      const { transport, tableId, yourSeat } = get();
      if (!transport || !tableId || yourSeat === null) return;

      const at = Date.now();

      // 1) Optimistic local preview (< 100ms, no spinner, no server wait).
      set((s) => ({
        pending: { kind, amount, at },
        seats: s.seats.map((seat) =>
          seat.seat === yourSeat
            ? { ...seat, lastAction: { kind, amount } }
            : seat,
        ),
        // Hand the action cursor away immediately so the timer clears locally.
        nextToAct: -1,
      }));

      // 2) Ship the command. The server confirms via bet_placed (clears
      //    pending) or rejects via error (rolls back + shakes).
      transport.send({
        type: Cmd.PlaceBet,
        data: { tableId, kind, amount },
      });

      // 3) Guard against a silently dropped confirmation: if nothing settles
      //    this preview within the timeout, drop it and resync from the
      //    server rather than trust a stale local guess.
      clearPendingTimer();
      pendingTimer = window.setTimeout(() => {
        pendingTimer = null;
        const cur = get();
        if (cur.pending?.at !== at) return; // already resolved by a newer action
        set((s) => ({
          pending: null,
          rollbackNonce: s.rollbackNonce + 1,
        }));
        if (cur.transport && cur.tableId) {
          cur.transport.send({
            type: Cmd.Resync,
            data: { tableId: cur.tableId, haveSeq: 0 },
          });
        }
      }, PENDING_ACTION_TIMEOUT_MS);
    },

    fold: () => get().act("fold", 0),
    check: () => get().act("check", 0),
    call: () => get().act("call", 0),
    bet: (amount) => get().act("bet", amount),
    raise: (amount) => get().act("raise", amount),

    clearError: () => set({ lastError: null }),

    startHand: () => {
      const { transport, tableId } = get();
      if (!transport || !tableId) return;
      transport.send({ type: Cmd.StartHand, data: { tableId } });
    },

    rebuy: (amount) => {
      const { transport, tableId } = get();
      if (!transport || !tableId || amount <= 0) return;
      set({ rebuyPending: true, rebuyError: null });
      transport.send({ type: Cmd.Rebuy, data: { tableId, amount } });
    },

    clearRebuyError: () => set({ rebuyError: null }),

    sitOut: () => {
      const { transport, tableId } = get();
      if (!transport || !tableId) return;
      transport.send({ type: Cmd.SitOut, data: { tableId } });
    },

    sitIn: () => {
      const { transport, tableId } = get();
      if (!transport || !tableId) return;
      transport.send({ type: Cmd.SitIn, data: { tableId } });
    },

    clearBlindsUpBanner: () =>
      set((s) => ({ tourney: { ...s.tourney, blindsUp: null } })),
    clearElimination: () =>
      set((s) => ({ tourney: { ...s.tourney, lastElimination: null } })),
    clearTourneyResult: () =>
      set((s) => ({ tourney: { ...s.tourney, result: null } })),

    getFairnessRecord: (handId) =>
      get().fairness.find((f) => f.handId === handId),
    getFairnessRecords: () => get().fairness,
  };
});
