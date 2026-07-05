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
import {
  Cmd,
  Ev,
  type BetKind,
  type Card,
  type SeatState,
  type ServerEvent,
  type Street,
  type TableSnapshot,
} from "@/net/protocol";

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

  // ---- optimistic layer ----
  pending: PendingAction | null;
  rollbackNonce: number; // bump to trigger the action-bar shake

  // ---- ancillary ----
  lastError: { code: string; message: string } | null;
  history: HandRecord[];
  reveals: Record<string, { commitment: string; seed: string }>;

  // ---- actions ----
  connect: (opts: { url?: string; mock?: boolean }) => void;
  disconnect: () => void;
  act: (kind: BetKind, amount: number) => void;
  clearError: () => void;
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
          return { seats, board: d.board, street: "showdown", nextToAct: -1 };
        });
        break;
      }
      case Ev.SeatUpdate: {
        const d = ev.data;
        set((s) => ({
          seats: s.seats.some((x) => x.seat === d.seat.seat)
            ? s.seats.map((x) => (x.seat === d.seat.seat ? d.seat : x))
            : [...s.seats, d.seat].sort((a, b) => a.seat - b.seat),
        }));
        break;
      }
      case Ev.FairReveal: {
        const d = ev.data;
        set((s) => ({
          reveals: {
            ...s.reveals,
            [d.handId]: { commitment: d.commitment, seed: d.seed },
          },
        }));
        break;
      }
      case Ev.Error: {
        const d = ev.data;
        // An error on our own pending action is a rollback: undo + shake.
        set((s) => ({
          lastError: d,
          pending: null,
          rollbackNonce: s.pending ? s.rollbackNonce + 1 : s.rollbackNonce,
        }));
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

  const handlers = {
    onEvent: applyEvent,
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

    connect: ({ url, mock }) => {
      get().transport?.close();
      if (mock || !url) {
        const server = new MockServer(handlers);
        set({ transport: server, usingMock: true });
        server.connect();
      } else {
        const client = new WsClient(url, handlers);
        set({ transport: client, usingMock: false });
        client.connect();
      }
    },

    disconnect: () => {
      get().transport?.close();
      set({ transport: null, status: "closed", ...EMPTY_TABLE });
    },

    act: (kind, amount) => {
      const { transport, tableId, yourSeat, seats } = get();
      if (!transport || !tableId || yourSeat === null) return;

      // 1) Optimistic local preview (< 100ms, no spinner, no server wait).
      set((s) => ({
        pending: { kind, amount, at: Date.now() },
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

      void seats; // referenced for clarity that we read current seats above
    },

    clearError: () => set({ lastError: null }),
  };
});
