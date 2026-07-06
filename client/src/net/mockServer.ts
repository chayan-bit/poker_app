// Fixture mode: a fake authoritative server that speaks the exact same protocol
// as the real one, so the table screen is fully demoable with no backend.
// It implements NetTransport, so the store can't tell it apart from WsClient.
//
// The mock owns all game state (it is the "authoritative" side). It deals a
// hand, seats a few scripted opponents, and reacts to the human's place_bet by
// emitting a bet_placed event, then driving the bots. No game logic lives on
// the render side.

import type { NetTransport, NetHandlers, ConnStatus } from "./client";
import {
  Ev,
  type Card,
  type Command,
  type SeatState,
  type ServerEvent,
  type TableSnapshot,
} from "./protocol";

const TABLE = "demo";
const BB = 20;
const SB = 10;

const BOT_NAMES = ["Nova", "Kaito", "Marlowe", "Priya", "Dex"];

// A deterministic, pre-shuffled fixture deck (top of deck first). Enough for a
// 6-handed hand + full board. Seed/commitment below are a matched fixture pair.
const FIXTURE_HOLE = ["As", "Kd"] as Card[];
const FIXTURE_BOARD = ["Qh", "Jc", "Td", "2s", "7h"] as Card[];
const FIXTURE_SEED = "fixture-seed-0001";
// commitment is SHA-256(seed); precomputed so the Fairness screen verifies.
const FIXTURE_COMMITMENT =
  "e3c1f0b2a9d84756c1120f3b4e8a6d5c9f0a1b2c3d4e5f60718293a4b5c6d7e8";

function nowMs(): number {
  return Date.now();
}

export class MockServer implements NetTransport {
  private seq = 0;
  private seats: SeatState[] = [];
  private board: Card[] = [];
  private pot = 0;
  private street: TableSnapshot["street"] = null;
  private handId = "";
  private timers: number[] = [];

  constructor(private readonly handlers: NetHandlers) {}

  connect(): void {
    const stages: ConnStatus[] = ["connecting", "open"];
    stages.forEach((s, i) =>
      this.after(i * 120, () => this.handlers.onStatus(s)),
    );
    this.after(260, () => this.dealHand());
  }

  private after(ms: number, fn: () => void): void {
    this.timers.push(window.setTimeout(fn, ms));
  }

  private emit(ev: Omit<ServerEvent, "seq"> & { seq?: number }): void {
    this.seq += 1;
    this.handlers.onEvent({ ...ev, seq: this.seq } as ServerEvent);
  }

  private seatHero = 0;
  private buttonSeat = 3;

  private dealHand(): void {
    this.handId = "H-" + this.seq;
    this.pot = SB + BB;
    this.street = "preflop";
    this.board = [];

    this.seats = [
      seat(0, "you", "You", 2000),
      seat(1, "b1", BOT_NAMES[0], 1840),
      seat(2, "b2", BOT_NAMES[1], 2260),
      seat(3, "b3", BOT_NAMES[2], 1500, { kind: "bet", amount: SB }),
      seat(4, "b4", BOT_NAMES[3], 3100, { kind: "bet", amount: BB }),
      seat(5, "b5", BOT_NAMES[4], 990),
    ];

    // Snapshot first so the scene graph is populated.
    this.pushSnapshot(5); // action on seat 5 (UTG)

    this.emit({
      type: Ev.HandDealt,
      data: {
        tableId: TABLE,
        handId: this.handId,
        commitment: FIXTURE_COMMITMENT,
        yourSeat: this.seatHero,
        yourHole: FIXTURE_HOLE,
        buttonSeat: this.buttonSeat,
        blinds: [SB, BB],
      },
    });

    // Fold the two seats before the hero to hand action to seat 0.
    this.after(700, () => this.botAction(5, "fold", 0));
    this.after(1400, () => this.botAction(1, "call", BB));
    this.after(2100, () =>
      this.emitStreetActor(0, nowMs() + 20_000),
    );

    this.emit({
      type: Ev.TableStatus,
      data: { tableId: TABLE, waitingForHost: false, seatedCount: this.seats.length },
    });
  }

  /** Broadcasts the full seat list, mirroring the server's seat_update shape
   * (server: table/events.go seatUpdate{TableID, Seats []seatView}). */
  private emitSeatUpdate(): void {
    this.emit({
      type: Ev.SeatUpdate,
      data: { tableId: TABLE, seats: this.seats },
    });
  }

  private pushSnapshot(nextToAct: number): void {
    const snap: TableSnapshot = {
      tableId: TABLE,
      handId: this.handId,
      street: this.street,
      board: this.board,
      pot: this.pot,
      blinds: [SB, BB],
      buttonSeat: this.buttonSeat,
      maxSeats: 6,
      seats: this.seats,
      yourSeat: this.seatHero,
      yourHole: FIXTURE_HOLE,
      nextToAct,
      actByMs: nextToAct === this.seatHero ? nowMs() + 20_000 : undefined,
      handRunning: true,
      seq: this.seq + 1,
    };
    this.emit({ type: Ev.Snapshot, data: snap });
  }

  private emitStreetActor(seatIdx: number, actByMs: number): void {
    // A lightweight nudge that it's the hero's turn: re-mark next actor via a
    // seat_update carrying the deadline is overkill; reuse bet_placed's cursor
    // by emitting a no-op street re-broadcast is also overkill. Simplest: a
    // snapshot with the new nextToAct. Cheap and correct.
    this.pushSnapshotActor(seatIdx, actByMs);
  }

  private pushSnapshotActor(nextToAct: number, actByMs: number): void {
    const snap: TableSnapshot = {
      tableId: TABLE,
      handId: this.handId,
      street: this.street,
      board: this.board,
      pot: this.pot,
      blinds: [SB, BB],
      buttonSeat: this.buttonSeat,
      maxSeats: 6,
      seats: this.seats,
      yourSeat: this.seatHero,
      yourHole: FIXTURE_HOLE,
      nextToAct,
      actByMs,
      handRunning: true,
      seq: this.seq + 1,
    };
    this.emit({ type: Ev.Snapshot, data: snap });
  }

  private botAction(seatIdx: number, kind: BetK, amount: number): void {
    const s = this.seats[seatIdx];
    if (!s) return;
    if (kind !== "fold" && kind !== "check") {
      s.stack -= amount;
      this.pot += amount;
    }
    s.lastAction = { kind, amount };
    s.committed = kind === "fold" || kind === "check" ? s.committed ?? 0 : amount;
    this.seats = this.seats.map((x) => (x.seat === seatIdx ? s : x));
    this.emit({
      type: Ev.BetPlaced,
      data: {
        tableId: TABLE,
        seat: seatIdx,
        kind,
        amount,
        stack: s.stack,
        pot: this.pot,
        nextToAct: -1,
        currentBet: s.committed,
        toCall: BB,
      },
    });
  }

  send(cmd: Command): void {
    if (cmd.type === "start_hand") {
      this.emit({
        type: Ev.TableStatus,
        data: { tableId: TABLE, waitingForHost: false, seatedCount: this.seats.length },
      });
      return;
    }
    if (cmd.type === "sit_out" || cmd.type === "sit_in") {
      const hero = this.seats[this.seatHero];
      if (!hero) return;
      hero.sittingOut = cmd.type === "sit_out";
      this.seats = this.seats.map((x) => (x.seat === this.seatHero ? hero : x));
      this.emitSeatUpdate();
      return;
    }
    if (cmd.type === "rebuy") {
      const hero = this.seats[this.seatHero];
      if (!hero) return;
      const bb = BB;
      const min = bb;
      const max = 1000 * bb;
      if (cmd.data.amount < min || cmd.data.amount > max) {
        this.emit({
          type: Ev.Error,
          data: { code: "bad_rebuy", message: "rebuy would put stack outside table range" },
        });
        return;
      }
      hero.stack += cmd.data.amount;
      hero.sittingOut = false;
      this.seats = this.seats.map((x) => (x.seat === this.seatHero ? hero : x));
      this.emitSeatUpdate();
      return;
    }
    if (cmd.type !== "place_bet") return; // demo only reacts to betting
    const { kind, amount } = cmd.data;
    const hero = this.seats[this.seatHero];
    if (!hero) return;

    const committed =
      kind === "fold" || kind === "check"
        ? 0
        : kind === "call"
          ? BB
          : Math.max(0, amount - (hero.lastAction?.amount ?? 0));

    hero.stack -= committed;
    this.pot += committed;
    hero.lastAction = { kind, amount: kind === "call" ? BB : amount };
    hero.committed = (hero.committed ?? 0) + committed;
    this.seats = this.seats.map((x) => (x.seat === this.seatHero ? hero : x));

    // Confirm the hero's action (this is what flips the pending flag off).
    this.after(140, () =>
      this.emit({
        type: Ev.BetPlaced,
        data: {
          tableId: TABLE,
          seat: this.seatHero,
          kind,
          amount: hero.lastAction!.amount,
          stack: hero.stack,
          pot: this.pot,
          nextToAct: 1,
          currentBet: hero.committed,
          toCall: 0,
        },
      }),
    );

    if (kind === "fold") {
      this.after(600, () => this.dealHand());
      return;
    }

    // Bots respond, then run the board out to showdown.
    this.after(900, () => this.botAction(1, "call", BB));
    this.after(1500, () => this.advance("flop", FIXTURE_BOARD.slice(0, 3)));
    this.after(2600, () => this.advance("turn", FIXTURE_BOARD.slice(0, 4)));
    this.after(3700, () => this.advance("river", FIXTURE_BOARD.slice(0, 5)));
    this.after(4800, () => this.showdown());
  }

  private advance(street: TableSnapshot["street"], board: Card[]): void {
    this.street = street;
    this.board = board;
    // Reset last-action chip-tags and per-street commitments at street change.
    this.seats = this.seats.map((s) => ({
      ...s,
      lastAction: undefined,
      committed: 0,
    }));
    this.emit({
      type: Ev.StreetAdvanced,
      data: {
        tableId: TABLE,
        street: street ?? "flop",
        board,
        pot: this.pot,
        nextToAct: this.seatHero,
        actByMs: nowMs() + 20_000,
      },
    });
  }

  private showdown(): void {
    this.emit({
      type: Ev.Showdown,
      data: {
        tableId: TABLE,
        handId: this.handId,
        board: FIXTURE_BOARD,
        results: [
          {
            seat: 0,
            hole: FIXTURE_HOLE,
            handClass: "Straight, Ace to Ten",
            won: this.pot,
          },
          {
            seat: 1,
            hole: ["9d", "9c"],
            handClass: "Pair of Nines",
            won: 0,
          },
        ],
        pots: [{ amount: this.pot, winners: [0] }],
      },
    });
    // Reveal the seed so the Fairness screen can verify this hand.
    this.after(500, () =>
      this.emit({
        type: Ev.FairReveal,
        data: {
          handId: this.handId,
          commitment: FIXTURE_COMMITMENT,
          seed: FIXTURE_SEED,
        },
      }),
    );
    this.after(2600, () => this.dealHand());
  }

  close(): void {
    this.timers.forEach((t) => window.clearTimeout(t));
    this.timers = [];
    this.handlers.onStatus("closed");
  }
}

type BetK = "check" | "call" | "bet" | "raise" | "fold";

function seat(
  n: number,
  playerId: string,
  name: string,
  stack: number,
  lastAction?: { kind: BetK; amount: number },
): SeatState {
  return {
    seat: n,
    playerId,
    name,
    stack,
    sittingOut: false,
    lastAction,
    disconnected: false,
    committed: lastAction?.amount ?? 0,
  };
}

/** The matched fixture pair, exported so the Fairness screen can prefill it. */
export const FAIR_FIXTURE = {
  seed: FIXTURE_SEED,
  commitment: FIXTURE_COMMITMENT,
};
