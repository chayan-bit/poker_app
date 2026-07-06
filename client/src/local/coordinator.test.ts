// Unit tests for the deterministic coordinator-successor math: every peer must
// compute the SAME coordinator seat from the same replicated inputs.

import { describe, expect, it } from "vitest";
import {
  coordinatorSeat,
  eligibleCount,
  nextOccupied,
  occupiedSeats,
  type SeatInfo,
} from "./coordinator";

function seat(playerId: string, stack = 1000, sittingOut = false): SeatInfo {
  return { playerId, stack, sittingOut };
}

function table(entries: [number, SeatInfo][]): Map<number, SeatInfo> {
  return new Map(entries);
}

describe("occupiedSeats", () => {
  it("returns occupied seat ids in ascending order regardless of insertion order", () => {
    const seats = table([
      [7, seat("g")],
      [1, seat("a")],
      [4, seat("d")],
    ]);
    expect(occupiedSeats(seats)).toEqual([1, 4, 7]);
  });

  it("returns an empty array for an empty table", () => {
    expect(occupiedSeats(table([]))).toEqual([]);
  });
});

describe("nextOccupied", () => {
  const seats = table([
    [1, seat("a")],
    [4, seat("d")],
    [7, seat("g")],
  ]);

  it("returns the next occupied seat strictly clockwise", () => {
    expect(nextOccupied(seats, 1)).toBe(4);
    expect(nextOccupied(seats, 4)).toBe(7);
  });

  it("wraps around from the highest seat back to the lowest", () => {
    expect(nextOccupied(seats, 7)).toBe(1);
    expect(nextOccupied(seats, 9)).toBe(1);
  });

  it("starts from gaps between seats without requiring the start to be occupied", () => {
    expect(nextOccupied(seats, 2)).toBe(4);
  });

  it("returns the same seat when it is the only occupied one", () => {
    expect(nextOccupied(table([[3, seat("solo")]]), 3)).toBe(3);
  });

  it("returns -1 when no seats are occupied", () => {
    expect(nextOccupied(table([]), 0)).toBe(-1);
  });
});

describe("coordinatorSeat", () => {
  const alive = (...ids: string[]) => new Set(ids);

  it("returns -1 for an empty table", () => {
    expect(coordinatorSeat(table([]), alive("a"), 0, false)).toBe(-1);
  });

  it("keeps the live dealer button as coordinator while a hand runs", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d")],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a", "d", "g"), 4, true)).toBe(4);
  });

  it("falls to the next eligible seat clockwise when the dealer dies mid-hand", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d")],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a", "g"), 4, true)).toBe(7);
  });

  it("picks the seat after the button between hands (the prospective button)", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d")],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a", "d", "g"), 4, false)).toBe(7);
  });

  it("wraps around the table when the button is the highest occupied seat", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d")],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a", "d", "g"), 7, false)).toBe(1);
  });

  it("skips busted seats when choosing the successor", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d", 0)],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a", "d", "g"), 1, false)).toBe(7);
  });

  it("skips sitting-out seats when choosing the successor", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d", 500, true)],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a", "d", "g"), 1, false)).toBe(7);
  });

  it("skips dead peers and wraps to the first eligible seat", () => {
    const seats = table([
      [1, seat("a")],
      [4, seat("d")],
      [7, seat("g")],
    ]);
    expect(coordinatorSeat(seats, alive("a"), 4, false)).toBe(1);
  });

  it("returns -1 when no seat is eligible", () => {
    const seats = table([
      [1, seat("a", 0)],
      [4, seat("d", 500, true)],
    ]);
    expect(coordinatorSeat(seats, alive("a", "d"), 1, false)).toBe(-1);
  });
});

describe("eligibleCount", () => {
  it("counts only alive, chipped, sitting-in seats", () => {
    const seats = table([
      [1, seat("a")],
      [2, seat("b", 0)],
      [3, seat("c", 500, true)],
      [4, seat("d")],
    ]);
    expect(eligibleCount(seats, new Set(["a", "b", "c"]))).toBe(1);
    expect(eligibleCount(seats, new Set(["a", "d"]))).toBe(2);
  });

  it("returns zero for an empty table", () => {
    expect(eligibleCount(table([]), new Set(["a"]))).toBe(0);
  });
});
