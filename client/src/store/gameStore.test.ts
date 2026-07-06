// @vitest-environment jsdom
//
// Store dispatch tests: drive the zustand store's NetHandlers with synthetic
// ServerEvents through connectLocal (the same sink the WS client uses) and
// assert the resulting state transitions. jsdom because the optimistic action
// path uses window.setTimeout for its pending-action guard.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { webcrypto } from "node:crypto";
import { useGame, type HandRecord } from "./gameStore";
import type { Command, SeatState, ServerEvent } from "@/net/protocol";
import { Cmd, Ev } from "@/net/protocol";
import type { NetHandlers } from "@/net/client";

// jsdom ships getRandomValues but not SubtleCrypto; the fairness verifier
// needs crypto.subtle.digest, so fall back to Node's WebCrypto if absent.
if (!globalThis.crypto?.subtle) {
  vi.stubGlobal("crypto", webcrypto);
}

/** SHA-256("deadbeef" as text), the commitment matching seed "deadbeef". */
const COMMIT_DEADBEEF = "2baf1f40105d9501fe319a8ec463fdf4325a2a5df445adf3f572f626253678c9";

function seatFixture(seat: number, name: string, stack: number): SeatState {
  return { seat, playerId: `${name}-id`, name, stack, sittingOut: false, disconnected: false };
}

function snapshotEvent(): ServerEvent {
  return {
    type: Ev.Snapshot,
    seq: 1,
    data: {
      tableId: "t1",
      handId: null,
      street: null,
      board: [],
      pot: 0,
      blinds: [5, 10],
      buttonSeat: 0,
      maxSeats: 6,
      seats: [seatFixture(1, "Hero", 1000), seatFixture(3, "Bob", 800)],
      yourSeat: 1,
      yourHole: [],
      nextToAct: -1,
      handRunning: false,
      seq: 1,
    },
  };
}

function handDealtEvent(handId: string): ServerEvent {
  return {
    type: Ev.HandDealt,
    seq: 2,
    data: {
      tableId: "t1",
      handId,
      commitment: COMMIT_DEADBEEF,
      yourSeat: 1,
      yourHole: ["As", "Kd"],
      buttonSeat: 3,
      blinds: [5, 10],
    },
  };
}

function resetStore(): void {
  useGame.setState({
    status: "closed",
    transport: null,
    usingMock: false,
    tableId: null,
    handId: null,
    street: null,
    board: [],
    pot: 0,
    blinds: [0, 0],
    buttonSeat: -1,
    maxSeats: 9,
    seats: [],
    yourSeat: null,
    yourHole: [],
    nextToAct: -1,
    actByMs: null,
    handRunning: false,
    pending: null,
    rollbackNonce: 0,
    lastError: null,
    history: [],
    reveals: {},
    fairness: [],
    tableStatus: null,
    rebuyPending: false,
    rebuyError: null,
    tourney: { level: 0, sb: 0, bb: 0, myPlace: null, blindsUp: null, lastElimination: null, result: null },
  });
}

describe("gameStore dispatch", () => {
  let handlers!: NetHandlers;
  let sent: Command[];

  beforeEach(() => {
    resetStore();
    sent = [];
    useGame.getState().connectLocal((h) => {
      handlers = h;
      return { send: (cmd) => sent.push(cmd), close: () => {} };
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  const dispatch = (ev: ServerEvent) => handlers.onEvent(ev);

  it("applies a table snapshot as ground truth and clears any optimistic preview", () => {
    useGame.setState({ pending: { kind: "call", amount: 0, at: 1 } });
    dispatch(snapshotEvent());
    const s = useGame.getState();
    expect(s.tableId).toBe("t1");
    expect(s.blinds).toEqual([5, 10]);
    expect(s.maxSeats).toBe(6);
    expect(s.seats.map((x) => x.seat)).toEqual([1, 3]);
    expect(s.yourSeat).toBe(1);
    expect(s.pending).toBeNull();
  });

  it("starts a hand on hand_dealt and seeds a new history record", () => {
    dispatch(snapshotEvent());
    dispatch(handDealtEvent("h1"));
    const s = useGame.getState();
    expect(s.handId).toBe("h1");
    expect(s.street).toBe("preflop");
    expect(s.board).toEqual([]);
    expect(s.handRunning).toBe(true);
    expect(s.yourHole).toEqual(["As", "Kd"]);
    expect(s.buttonSeat).toBe(3);
    expect(s.history[0].handId).toBe("h1");
    expect(s.history[0].commitment).toBe(COMMIT_DEADBEEF);
  });

  it("updates the actor's stack, the pot, and nextToAct on bet_placed", () => {
    dispatch(snapshotEvent());
    dispatch({
      type: Ev.BetPlaced,
      seq: 3,
      data: { tableId: "t1", seat: 3, kind: "bet", amount: 50, stack: 750, pot: 65, nextToAct: 1 },
    });
    const s = useGame.getState();
    const bob = s.seats.find((x) => x.seat === 3);
    expect(bob?.stack).toBe(750);
    expect(bob?.lastAction).toEqual({ kind: "bet", amount: 50 });
    expect(s.pot).toBe(65);
    expect(s.nextToAct).toBe(1);
  });

  it("clears the pending preview when the server confirms your own bet", () => {
    dispatch(snapshotEvent());
    useGame.setState({ pending: { kind: "call", amount: 0, at: 1 } });
    dispatch({
      type: Ev.BetPlaced,
      seq: 3,
      data: { tableId: "t1", seat: 1, kind: "call", amount: 10, stack: 990, pot: 25, nextToAct: 3 },
    });
    expect(useGame.getState().pending).toBeNull();
  });

  it("keeps the pending preview when another seat's bet is confirmed", () => {
    dispatch(snapshotEvent());
    useGame.setState({ pending: { kind: "call", amount: 0, at: 1 } });
    dispatch({
      type: Ev.BetPlaced,
      seq: 3,
      data: { tableId: "t1", seat: 3, kind: "call", amount: 10, stack: 790, pot: 25, nextToAct: 1 },
    });
    expect(useGame.getState().pending).not.toBeNull();
  });

  it("advances the street, reveals the board, and clears lastAction tags", () => {
    dispatch(snapshotEvent());
    dispatch({
      type: Ev.BetPlaced,
      seq: 3,
      data: { tableId: "t1", seat: 3, kind: "bet", amount: 50, stack: 750, pot: 65, nextToAct: 1 },
    });
    dispatch({
      type: Ev.StreetAdvanced,
      seq: 4,
      data: { tableId: "t1", street: "flop", board: ["2c", "7d", "Jh"], pot: 65, nextToAct: 3 },
    });
    const s = useGame.getState();
    expect(s.street).toBe("flop");
    expect(s.board).toEqual(["2c", "7d", "Jh"]);
    expect(s.seats.every((x) => x.lastAction === undefined)).toBe(true);
  });

  it("credits winners and ends the hand on showdown", () => {
    dispatch(snapshotEvent());
    dispatch(handDealtEvent("h1"));
    dispatch({
      type: Ev.Showdown,
      seq: 5,
      data: {
        tableId: "t1",
        handId: "h1",
        board: ["2c", "7d", "Jh", "Qs", "3c"],
        results: [{ seat: 3, hole: ["9c", "9d"], handClass: "Pair of Nines", won: 120 }],
        pots: [{ amount: 120, winners: [3] }],
      },
    });
    const s = useGame.getState();
    expect(s.seats.find((x) => x.seat === 3)?.stack).toBe(800 + 120);
    expect(s.street).toBe("showdown");
    expect(s.nextToAct).toBe(-1);
    expect(s.handRunning).toBe(false);
  });

  it("replaces the full seat list sorted by seat on seat_update", () => {
    dispatch({
      type: Ev.SeatUpdate,
      seq: 2,
      data: { tableId: "t1", seats: [seatFixture(5, "Eve", 400), seatFixture(2, "Dan", 600)] },
    });
    expect(useGame.getState().seats.map((x) => x.seat)).toEqual([2, 5]);
  });

  it("settles an in-flight rebuy when the next seat_update lands", () => {
    useGame.setState({ rebuyPending: true, rebuyError: "old" });
    dispatch({
      type: Ev.SeatUpdate,
      seq: 2,
      data: { tableId: "t1", seats: [seatFixture(1, "Hero", 500)] },
    });
    const s = useGame.getState();
    expect(s.rebuyPending).toBe(false);
    expect(s.rebuyError).toBeNull();
  });

  it("records a fair reveal and auto-verifies a matching commitment", async () => {
    dispatch({
      type: Ev.FairReveal,
      seq: 6,
      data: { handId: "h1", commitment: COMMIT_DEADBEEF, seed: "deadbeef" },
    });
    expect(useGame.getState().reveals["h1"]).toEqual({
      commitment: COMMIT_DEADBEEF,
      seed: "deadbeef",
    });
    await vi.waitFor(() => {
      expect(useGame.getState().getFairnessRecord("h1")?.verified).toBe(true);
    });
  });

  it("flags a fair reveal whose seed does not match the commitment", async () => {
    dispatch({
      type: Ev.FairReveal,
      seq: 6,
      data: { handId: "h2", commitment: "00".repeat(32), seed: "deadbeef" },
    });
    await vi.waitFor(() => {
      expect(useGame.getState().getFairnessRecord("h2")?.verified).toBe(false);
    });
    expect(useGame.getState().getFairnessRecords()).toHaveLength(1);
  });

  it("surfaces the waiting-for-host banner from table_status", () => {
    dispatch({
      type: Ev.TableStatus,
      seq: 2,
      data: { tableId: "t1", waitingForHost: true, seatedCount: 2 },
    });
    expect(useGame.getState().tableStatus).toEqual({ waitingForHost: true, seatedCount: 2 });
  });

  it("rolls back a pending action and bumps the shake nonce on error", () => {
    useGame.setState({ pending: { kind: "raise", amount: 50, at: 1 } });
    dispatch({ type: Ev.Error, data: { code: "bad_bet", message: "not your turn" } });
    const s = useGame.getState();
    expect(s.pending).toBeNull();
    expect(s.lastError).toEqual({ code: "bad_bet", message: "not your turn" });
    expect(s.rollbackNonce).toBe(1);
    s.clearError();
    expect(useGame.getState().lastError).toBeNull();
  });

  it("does not shake when an error arrives with no pending action", () => {
    dispatch({ type: Ev.Error, data: { code: "oops", message: "x" } });
    expect(useGame.getState().rollbackNonce).toBe(0);
  });

  it("routes an error into the rebuy sheet while a rebuy is pending", () => {
    dispatch(snapshotEvent());
    useGame.getState().rebuy(500);
    expect(useGame.getState().rebuyPending).toBe(true);
    expect(sent).toContainEqual({ type: Cmd.Rebuy, data: { tableId: "t1", amount: 500 } });
    dispatch({ type: Ev.Error, data: { code: "table_full", message: "cannot rebuy now" } });
    const s = useGame.getState();
    expect(s.rebuyPending).toBe(false);
    expect(s.rebuyError).toBe("cannot rebuy now");
    expect(s.lastError).toBeNull();
    s.clearRebuyError();
    expect(useGame.getState().rebuyError).toBeNull();
  });

  it("updates the tourney slice and the header blinds on blinds_up", () => {
    dispatch({ type: Ev.BlindsUp, seq: 7, data: { level: 3, sb: 50, bb: 100 } });
    const s = useGame.getState();
    expect(s.tourney.level).toBe(3);
    expect(s.blinds).toEqual([50, 100]);
    expect(s.tourney.blindsUp).toMatchObject({ level: 3, sb: 50, bb: 100 });
    s.clearBlindsUpBanner();
    expect(useGame.getState().tourney.blindsUp).toBeNull();
  });

  it("records an elimination with the busted seat's last-known name", () => {
    dispatch(snapshotEvent());
    dispatch({ type: Ev.Elimination, seq: 8, data: { seat: 3, playerId: "Bob-id", place: 4 } });
    const s = useGame.getState();
    expect(s.tourney.lastElimination).toMatchObject({ seat: 3, place: 4, name: "Bob" });
    expect(s.tourney.myPlace).toBeNull();
    s.clearElimination();
    expect(useGame.getState().tourney.lastElimination).toBeNull();
  });

  it("sets your final place when the eliminated seat is yours", () => {
    dispatch(snapshotEvent());
    dispatch({ type: Ev.Elimination, seq: 8, data: { seat: 1, playerId: "Hero-id", place: 2 } });
    expect(useGame.getState().tourney.myPlace).toBe(2);
  });

  it("stores the final standings from tourney_result", () => {
    const places = [{ playerId: "Hero-id", place: 1, prize: 900 }];
    dispatch({ type: Ev.TourneyResult, seq: 9, data: { places } });
    expect(useGame.getState().tourney.result).toEqual({ places });
    useGame.getState().clearTourneyResult();
    expect(useGame.getState().tourney.result).toBeNull();
  });

  it("caps the live hand history at 50 records, newest first", () => {
    for (let i = 1; i <= 55; i++) dispatch(handDealtEvent(`h${i}`));
    const history = useGame.getState().history;
    expect(history).toHaveLength(50);
    expect(history[0].handId).toBe("h55");
    expect(history[49].handId).toBe("h6");
  });

  it("appends follow-up events to the current hand's history record", () => {
    dispatch(handDealtEvent("h1"));
    dispatch({
      type: Ev.Showdown,
      seq: 5,
      data: { tableId: "t1", handId: "h1", board: ["2c", "7d", "Jh"], results: [], pots: [] },
    });
    const rec = useGame.getState().history[0];
    expect(rec.events).toHaveLength(2);
    expect(rec.board).toEqual(["2c", "7d", "Jh"]);
  });

  it("dedupes loadReplayHand by handId and puts the loaded record first", () => {
    dispatch(handDealtEvent("hA"));
    dispatch(handDealtEvent("hB"));
    const fetched: HandRecord = { handId: "hA", board: ["2c", "7d", "Jh"], events: [] };
    useGame.getState().loadReplayHand(fetched);
    const history = useGame.getState().history;
    expect(history).toHaveLength(2);
    expect(history[0]).toEqual(fetched);
    expect(history[1].handId).toBe("hB");
  });

  it("previews your action optimistically and ships place_bet", () => {
    dispatch(snapshotEvent());
    useGame.getState().bet(40);
    const s = useGame.getState();
    expect(s.pending).toMatchObject({ kind: "bet", amount: 40 });
    expect(s.seats.find((x) => x.seat === 1)?.lastAction).toEqual({ kind: "bet", amount: 40 });
    expect(s.nextToAct).toBe(-1);
    expect(sent).toContainEqual({
      type: Cmd.PlaceBet,
      data: { tableId: "t1", kind: "bet", amount: 40 },
    });
    // The confirming bet_placed settles the preview.
    dispatch({
      type: Ev.BetPlaced,
      seq: 3,
      data: { tableId: "t1", seat: 1, kind: "bet", amount: 40, stack: 960, pot: 55, nextToAct: 3 },
    });
    expect(useGame.getState().pending).toBeNull();
  });

  it("maps the fold/check/call/raise helpers onto place_bet kinds", () => {
    dispatch(snapshotEvent());
    const s = useGame.getState();
    s.fold();
    s.check();
    s.call();
    s.raise(80);
    const kinds = sent
      .filter((c) => c.type === Cmd.PlaceBet)
      .map((c) => (c.data as { kind: string }).kind);
    expect(kinds).toEqual(["fold", "check", "call", "raise"]);
  });

  it("ignores actions while unseated or disconnected", () => {
    useGame.getState().bet(40); // no tableId, no seat
    expect(sent).toHaveLength(0);
    expect(useGame.getState().pending).toBeNull();
  });

  it("rolls back and resyncs when a pending action is never confirmed", () => {
    vi.useFakeTimers();
    dispatch(snapshotEvent());
    useGame.getState().bet(40);
    vi.advanceTimersByTime(3000);
    const s = useGame.getState();
    expect(s.pending).toBeNull();
    expect(s.rollbackNonce).toBe(1);
    expect(sent).toContainEqual({ type: Cmd.Resync, data: { tableId: "t1", haveSeq: 0 } });
  });

  it("requests a resync when the client detects a seq gap", () => {
    dispatch(snapshotEvent());
    handlers.onGap(42);
    expect(sent).toContainEqual({ type: Cmd.Resync, data: { tableId: "t1", haveSeq: 42 } });
  });

  it("tracks connection status changes from the transport", () => {
    handlers.onStatus("open");
    expect(useGame.getState().status).toBe("open");
    handlers.onStatus("reconnecting");
    expect(useGame.getState().status).toBe("reconnecting");
  });

  it("sends start_hand, sit_out and sit_in for the current table", () => {
    dispatch(snapshotEvent());
    const s = useGame.getState();
    s.startHand();
    s.sitOut();
    s.sitIn();
    expect(sent).toContainEqual({ type: Cmd.StartHand, data: { tableId: "t1" } });
    expect(sent).toContainEqual({ type: Cmd.SitOut, data: { tableId: "t1" } });
    expect(sent).toContainEqual({ type: Cmd.SitIn, data: { tableId: "t1" } });
  });

  it("ignores a rebuy for a non-positive amount", () => {
    dispatch(snapshotEvent());
    useGame.getState().rebuy(0);
    expect(sent.filter((c) => c.type === Cmd.Rebuy)).toHaveLength(0);
  });

  it("resets the table but keeps history on disconnect", () => {
    dispatch(snapshotEvent());
    dispatch(handDealtEvent("h1"));
    useGame.getState().disconnect();
    const s = useGame.getState();
    expect(s.status).toBe("closed");
    expect(s.transport).toBeNull();
    expect(s.tableId).toBeNull();
    expect(s.seats).toEqual([]);
    expect(s.history).toHaveLength(1);
  });
});
