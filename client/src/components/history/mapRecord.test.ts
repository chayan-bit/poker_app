// Unit tests for the pure history mapper: Go-shaped ApiHandRecord in,
// store-shaped HandRecord / hole-card arrays out.

import { describe, expect, it } from "vitest";
import { isMaskedHole, mapToHandRecord, mapToHoleCards } from "./mapRecord";
import { Ev } from "@/net/protocol";
import type { ApiHandRecord } from "@/net/hands";

function baseRecord(overrides: Partial<ApiHandRecord> = {}): ApiHandRecord {
  return {
    HandID: "h-42",
    TableID: "t-9",
    StartedAt: "2026-07-01T12:00:00Z",
    ButtonSeat: 2,
    Blinds: [5, 10],
    Commitment: "c0ffee",
    SeedHex: "deadbeef",
    Seats: [
      { SeatID: 1, PlayerID: "alice", StartStack: 1000, Hole: ["As", "Kd"] },
      { SeatID: 2, PlayerID: "bob", StartStack: 800, Hole: ["??", "??"] },
    ],
    Events: [
      { Street: "preflop", SeatID: 1, Kind: "raise", Amount: 30 },
      { Street: "flop", SeatID: 0, Kind: "street", Amount: 0 },
      { Street: "turn", SeatID: 0, Kind: "street", Amount: 0 },
      { Street: "river", SeatID: 0, Kind: "street", Amount: 0 },
    ],
    Board: ["2c", "7d", "Jh", "Qs", "3c"],
    Awards: [{ SeatID: 1, Amount: 300 }],
    Results: { "1": "won 300 with Two Pair" },
    ...overrides,
  };
}

describe("isMaskedHole", () => {
  it("returns true for the server's masked sentinel", () => {
    expect(isMaskedHole(["??", "??"])).toBe(true);
  });

  it("returns false for visible hole cards", () => {
    expect(isMaskedHole(["As", "Kd"])).toBe(false);
  });

  it("returns false when only one card is masked", () => {
    expect(isMaskedHole(["??", "Kd"])).toBe(false);
    expect(isMaskedHole(["As", "??"])).toBe(false);
  });

  it("returns false when the length is not exactly two", () => {
    expect(isMaskedHole([])).toBe(false);
    expect(isMaskedHole(["??"])).toBe(false);
    expect(isMaskedHole(["??", "??", "??"])).toBe(false);
  });
});

describe("mapToHoleCards", () => {
  it("maps the masked sentinel to two face-down (undefined) cards", () => {
    expect(mapToHoleCards(["??", "??"])).toEqual([undefined, undefined]);
  });

  it("passes visible hole cards through unchanged", () => {
    expect(mapToHoleCards(["As", "Kd"])).toEqual(["As", "Kd"]);
  });
});

describe("mapToHandRecord", () => {
  it("maps top-level identity, board, commitment and seed fields", () => {
    const rec = mapToHandRecord(baseRecord());
    expect(rec.handId).toBe("h-42");
    expect(rec.board).toEqual(["2c", "7d", "Jh", "Qs", "3c"]);
    expect(rec.commitment).toBe("c0ffee");
    expect(rec.seed).toBe("deadbeef");
  });

  it("opens the timeline with a hand_dealt carrying button, blinds and commitment", () => {
    const rec = mapToHandRecord(baseRecord());
    const first = rec.events[0];
    expect(first.type).toBe(Ev.HandDealt);
    expect(first.seq).toBe(0);
    if (first.type !== Ev.HandDealt) throw new Error("expected hand_dealt");
    expect(first.data.tableId).toBe("t-9");
    expect(first.data.handId).toBe("h-42");
    expect(first.data.commitment).toBe("c0ffee");
    expect(first.data.buttonSeat).toBe(2);
    expect(first.data.blinds).toEqual([5, 10]);
  });

  it("leaves the viewer's seat and hole honestly empty (API has no viewer context)", () => {
    const rec = mapToHandRecord(baseRecord());
    const first = rec.events[0];
    if (first.type !== Ev.HandDealt) throw new Error("expected hand_dealt");
    expect(first.data.yourSeat).toBe(-1);
    expect(first.data.yourHole).toEqual([]);
  });

  it("emits one street_advanced per Kind=street event with the cumulative board", () => {
    const rec = mapToHandRecord(baseRecord());
    const streets = rec.events.filter((e) => e.type === Ev.StreetAdvanced);
    expect(streets).toHaveLength(3);
    expect(streets.map((e) => e.seq)).toEqual([1, 2, 3]);
    if (streets[0].type !== Ev.StreetAdvanced) throw new Error("expected street_advanced");
    expect(streets[0].data.street).toBe("flop");
    expect(streets[0].data.board).toEqual(["2c", "7d", "Jh"]);
    if (streets[1].type !== Ev.StreetAdvanced) throw new Error("expected street_advanced");
    expect(streets[1].data.board).toEqual(["2c", "7d", "Jh", "Qs"]);
    if (streets[2].type !== Ev.StreetAdvanced) throw new Error("expected street_advanced");
    expect(streets[2].data.board).toEqual(["2c", "7d", "Jh", "Qs", "3c"]);
  });

  it("ignores non-street action events when building the street timeline", () => {
    const rec = mapToHandRecord(baseRecord());
    const types = rec.events.map((e) => e.type);
    expect(types.filter((t) => t === Ev.BetPlaced)).toHaveLength(0);
  });

  it("never advances the cumulative board past the recorded board length", () => {
    const rec = mapToHandRecord(baseRecord({ Board: ["2c", "7d", "Jh", "Qs"] }));
    const streets = rec.events.filter((e) => e.type === Ev.StreetAdvanced);
    if (streets[2].type !== Ev.StreetAdvanced) throw new Error("expected street_advanced");
    expect(streets[2].data.board).toEqual(["2c", "7d", "Jh", "Qs"]);
  });

  it("builds showdown rows: visible holes shown, masked (mucked) holes empty", () => {
    const rec = mapToHandRecord(baseRecord());
    const showdown = rec.events.find((e) => e.type === Ev.Showdown);
    if (showdown?.type !== Ev.Showdown) throw new Error("expected showdown");
    const bySeat = new Map(showdown.data.results.map((r) => [r.seat, r]));
    expect(bySeat.get(1)?.hole).toEqual(["As", "Kd"]);
    expect(bySeat.get(2)?.hole).toEqual([]);
  });

  it("carries per-seat winnings and the free-text result into the showdown rows", () => {
    const rec = mapToHandRecord(baseRecord());
    const showdown = rec.events.find((e) => e.type === Ev.Showdown);
    if (showdown?.type !== Ev.Showdown) throw new Error("expected showdown");
    const bySeat = new Map(showdown.data.results.map((r) => [r.seat, r]));
    expect(bySeat.get(1)?.won).toBe(300);
    expect(bySeat.get(1)?.handClass).toBe("won 300 with Two Pair");
    expect(bySeat.get(2)?.won).toBe(0);
    expect(bySeat.get(2)?.handClass).toBe("");
  });

  it("sums multiple awards to the same seat and lists all pot winners", () => {
    const rec = mapToHandRecord(
      baseRecord({
        Awards: [
          { SeatID: 1, Amount: 200 },
          { SeatID: 1, Amount: 100 },
          { SeatID: 2, Amount: 50 },
        ],
      }),
    );
    const showdown = rec.events.find((e) => e.type === Ev.Showdown);
    if (showdown?.type !== Ev.Showdown) throw new Error("expected showdown");
    const bySeat = new Map(showdown.data.results.map((r) => [r.seat, r]));
    expect(bySeat.get(1)?.won).toBe(300);
    expect(bySeat.get(2)?.won).toBe(50);
    expect(showdown.data.pots).toEqual([{ amount: 350, winners: [1, 1, 2] }]);
  });

  it("appends a fair_reveal event when the seed has been revealed", () => {
    const rec = mapToHandRecord(baseRecord());
    const last = rec.events[rec.events.length - 1];
    expect(last.type).toBe(Ev.FairReveal);
    if (last.type !== Ev.FairReveal) throw new Error("expected fair_reveal");
    expect(last.data).toEqual({ handId: "h-42", commitment: "c0ffee", seed: "deadbeef" });
  });

  it("omits fair_reveal and leaves seed undefined when the seed is not yet revealed", () => {
    const rec = mapToHandRecord(baseRecord({ SeedHex: "" }));
    expect(rec.seed).toBeUndefined();
    expect(rec.events.some((e) => e.type === Ev.FairReveal)).toBe(false);
  });
});
