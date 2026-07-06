// Tests for the wire log-entry shapes (wire.ts) and the replicated view
// derivation (meshview.ts): the mesh learns the seat map, the button, and hand
// liveness purely from the event stream each applied log entry returns.

import { describe, expect, it } from "vitest";
import { BROADCAST, type Envelope, type EventMap } from "./core";
import { newView, updateView, type MeshView } from "./meshview";
import type { LogEntry, MeshMsg, Snapshot } from "./wire";

const SELF = "p1";

function broadcast(...envs: Envelope[]): EventMap {
  return { [BROADCAST]: envs };
}

function ev(type: string, data?: unknown): Envelope {
  return { v: 1, type, data };
}

function seatUpdate(
  seats: { seat: number; playerId: string; stack: number; sittingOut?: boolean; inHand?: boolean }[],
): Envelope {
  return ev("seat_update", {
    seats: seats.map((s) => ({ sittingOut: false, inHand: false, ...s })),
  });
}

describe("wire log entry shapes", () => {
  it("survives a JSON round-trip with every field intact for each entry kind", () => {
    const entries: LogEntry[] = [
      {
        seq: 1,
        kind: "seed",
        actor: "",
        seedHex: "ab".repeat(32),
        hashAfter: "h1",
        by: "p1",
      },
      {
        seq: 2,
        kind: "submit",
        actor: "p2",
        envelope: { v: 1, type: "place_bet", data: { tableId: "t1", kind: "call", amount: 0 } },
        hashAfter: "h2",
        by: "p1",
      },
      { seq: 3, kind: "tick", actor: "", nowMs: 12_345, hashAfter: "h3", by: "p1" },
      { seq: 4, kind: "void", actor: "p1", hashAfter: "h4", by: "p1" },
    ];
    const decoded = JSON.parse(JSON.stringify(entries)) as LogEntry[];
    expect(decoded).toEqual(entries);
    expect(decoded.map((e) => e.seq)).toEqual([1, 2, 3, 4]);
    expect(decoded.map((e) => e.kind)).toEqual(["seed", "submit", "tick", "void"]);
  });

  it("keeps snapshot and mesh frames JSON-serializable with their tags", () => {
    const snap: Snapshot = {
      config: { id: "t1", smallBlind: 1, bigBlind: 2 },
      entries: [],
      head: 0,
      stateHash: "h0",
    };
    const frames: MeshMsg[] = [
      { t: "hello", from: "p2" },
      { t: "heartbeat", from: "p2", head: 4, nowMs: 99, coordSeat: 1 },
      { t: "need", from: "p2", have: 2 },
      { t: "snapshot", from: "p2", snap },
    ];
    const decoded = JSON.parse(JSON.stringify(frames)) as MeshMsg[];
    expect(decoded).toEqual(frames);
    expect(decoded.map((f) => f.t)).toEqual(["hello", "heartbeat", "need", "snapshot"]);
  });
});

describe("newView", () => {
  it("starts empty with no button, no live hand, and nobody to act", () => {
    expect(newView()).toEqual({
      seats: new Map(),
      buttonSeat: -1,
      handRunning: false,
      toActSeat: -1,
      toCall: 0,
    });
  });
});

describe("updateView", () => {
  it("replaces the whole seat map from a seat_update broadcast", () => {
    const view = newView();
    updateView(
      view,
      broadcast(
        seatUpdate([
          { seat: 1, playerId: "alice", stack: 1000 },
          { seat: 3, playerId: "bob", stack: 800, sittingOut: true },
        ]),
      ),
      SELF,
    );
    expect([...view.seats.keys()]).toEqual([1, 3]);
    expect(view.seats.get(1)).toEqual({
      playerId: "alice",
      stack: 1000,
      sittingOut: false,
      inHand: false,
    });
    expect(view.seats.get(3)?.sittingOut).toBe(true);

    // A later seat_update replaces, not merges.
    updateView(view, broadcast(seatUpdate([{ seat: 5, playerId: "carol", stack: 400 }])), SELF);
    expect([...view.seats.keys()]).toEqual([5]);
  });

  it("marks the hand live and records the button from a hand_dealt privacy send", () => {
    const view = newView();
    // hand_dealt arrives on the recipient's own key, not the broadcast key.
    updateView(view, { [SELF]: [ev("hand_dealt", { buttonSeat: 2 })] }, SELF);
    expect(view.handRunning).toBe(true);
    expect(view.buttonSeat).toBe(2);
  });

  it("ignores events addressed to a different player's privacy stream", () => {
    const view = newView();
    updateView(view, { p9: [ev("hand_dealt", { buttonSeat: 2 })] }, SELF);
    expect(view.handRunning).toBe(false);
    expect(view.buttonSeat).toBe(-1);
  });

  it("tracks the seat to act and the amount to call from bet_placed", () => {
    const view = newView();
    updateView(view, broadcast(ev("bet_placed", { toAct: 3, toCall: 40 })), SELF);
    expect(view.toActSeat).toBe(3);
    expect(view.toCall).toBe(40);
  });

  it("resets the action cursor when a new street opens", () => {
    const view = newView();
    updateView(view, broadcast(ev("bet_placed", { toAct: 3, toCall: 40 })), SELF);
    updateView(view, broadcast(ev("street_advanced", { street: "flop" })), SELF);
    expect(view.toActSeat).toBe(-1);
    expect(view.toCall).toBe(0);
  });

  it("ends the hand on showdown", () => {
    const view = newView();
    updateView(view, { [SELF]: [ev("hand_dealt", { buttonSeat: 1 })] }, SELF);
    updateView(view, broadcast(ev("showdown", {})), SELF);
    expect(view.handRunning).toBe(false);
    expect(view.toActSeat).toBe(-1);
  });

  it("returns the event types it saw, broadcast stream first", () => {
    const view = newView();
    const seen = updateView(
      view,
      {
        [BROADCAST]: [ev("seat_update", { seats: [] }), ev("bet_placed", { toAct: 1 })],
        [SELF]: [ev("hand_dealt", { buttonSeat: 0 })],
      },
      SELF,
    );
    expect(seen).toEqual(["seat_update", "bet_placed", "hand_dealt"]);
  });

  it("leaves the view untouched by unknown event types", () => {
    const view = newView();
    const before: MeshView = { ...view, seats: new Map(view.seats) };
    const seen = updateView(view, broadcast(ev("chat_message", { text: "gl" })), SELF);
    expect(seen).toEqual(["chat_message"]);
    expect(view).toEqual(before);
  });

  it("derives the full table picture from a short synthetic hand sequence", () => {
    const view = newView();

    // 1. Two players sit down.
    updateView(
      view,
      broadcast(
        seatUpdate([
          { seat: 0, playerId: "alice", stack: 1000 },
          { seat: 1, playerId: "bob", stack: 1000 },
        ]),
      ),
      SELF,
    );
    expect(view.handRunning).toBe(false);

    // 2. A hand is dealt with the button on seat 0.
    updateView(view, { [SELF]: [ev("hand_dealt", { buttonSeat: 0 })] }, SELF);
    expect(view.handRunning).toBe(true);
    expect(view.buttonSeat).toBe(0);
    expect(view.toActSeat).toBe(-1);

    // 3. Preflop action announces the next actor.
    updateView(view, broadcast(ev("bet_placed", { toAct: 1, toCall: 2 })), SELF);
    expect(view.toActSeat).toBe(1);
    expect(view.toCall).toBe(2);

    // 4. The flop reopens betting with no announced first actor.
    updateView(view, broadcast(ev("street_advanced", { street: "flop" })), SELF);
    expect(view.toActSeat).toBe(-1);
    expect(view.toCall).toBe(0);

    // 5. Showdown ends the hand; seats persist for the next one.
    updateView(view, broadcast(ev("showdown", {})), SELF);
    expect(view.handRunning).toBe(false);
    expect(view.seats.size).toBe(2);
    expect(view.buttonSeat).toBe(0);
  });
});
