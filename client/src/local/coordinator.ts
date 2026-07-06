// Pure coordinator-successor math (issue #28). No I/O, no clock: given the
// replicated seat map, the liveness set, and the button, every peer computes
// the SAME coordinator seat with no election messages. "Clockwise" is ascending
// seat id wrapping back to the lowest occupied seat.

/** One seat's replicated public state, as tracked from seat_update events. */
export interface SeatInfo {
  playerId: string;
  stack: number;
  sittingOut: boolean;
}

/** Occupied seat ids in ascending (clockwise) order. */
export function occupiedSeats(seats: Map<number, SeatInfo>): number[] {
  return [...seats.keys()].sort((a, b) => a - b);
}

/** The next occupied seat strictly clockwise after `seat` (wraps). -1 if none. */
export function nextOccupied(seats: Map<number, SeatInfo>, seat: number): number {
  const ids = occupiedSeats(seats);
  if (ids.length === 0) return -1;
  for (const id of ids) if (id > seat) return id;
  return ids[0];
}

/** Rotate ascending ids so iteration begins at the first id >= start (wraps). */
function fromStart(ids: number[], start: number): number[] {
  let idx = ids.findIndex((id) => id >= start);
  if (idx < 0) idx = 0;
  return [...ids.slice(idx), ...ids.slice(0, idx)];
}

/**
 * A seat is coordinator-eligible when its player is alive and it can actually
 * deal: seated with chips and not sitting out. A busted or sitting-out seat is
 * skipped so the successor is always a peer that can drive a hand.
 */
function eligible(info: SeatInfo, alive: Set<string>): boolean {
  return alive.has(info.playerId) && info.stack > 0 && !info.sittingOut;
}

/**
 * The coordinator seat every peer agrees on.
 *
 * - During a hand it is the dealer button seat, UNLESS that peer is not alive,
 *   in which case it is the next eligible seat clockwise (the successor that
 *   will void the in-flight hand and start the next one).
 * - Between hands it is the next eligible seat clockwise after the button (the
 *   prospective button for the coming hand), skipping dead/ineligible seats.
 *
 * Returns -1 when no seat is eligible (game cannot proceed until someone is).
 */
export function coordinatorSeat(
  seats: Map<number, SeatInfo>,
  alive: Set<string>,
  buttonSeat: number,
  handRunning: boolean,
): number {
  const ids = occupiedSeats(seats);
  if (ids.length === 0) return -1;
  const start = handRunning ? buttonSeat : nextOccupied(seats, buttonSeat);
  for (const id of fromStart(ids, start)) {
    const info = seats.get(id)!;
    if (eligible(info, alive)) return id;
  }
  return -1;
}

/** Count of eligible seats; the coordinator needs >= 2 to deal. */
export function eligibleCount(seats: Map<number, SeatInfo>, alive: Set<string>): number {
  let n = 0;
  for (const info of seats.values()) if (eligible(info, alive)) n++;
  return n;
}
