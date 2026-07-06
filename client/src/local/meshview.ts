// Replicated view derived from the core's event stream (issue #28). The mesh
// never reads core-internal fields; it learns the seat map, the dealer button,
// and whether a hand is live purely from the events each applied log entry
// returns, exactly the data an online client renders from. This keeps the
// coordinator-successor computation a pure function of replicated state.

import { BROADCAST, type Envelope, type EventMap } from "./core.ts";
import type { SeatInfo } from "./coordinator.ts";

/** A seat as tracked from seat_update, with the in-hand flag the bot needs. */
export interface ViewSeat extends SeatInfo {
  inHand: boolean;
}

/** The mesh's replicated projection of the table. */
export interface MeshView {
  seats: Map<number, ViewSeat>;
  buttonSeat: number;
  handRunning: boolean;
  /** Seat currently to act, when announced by a bet_placed (-1 otherwise). */
  toActSeat: number;
  /** Chips the seat to act must add to call, when announced (0 otherwise). */
  toCall: number;
}

export function newView(): MeshView {
  return { seats: new Map(), buttonSeat: -1, handRunning: false, toActSeat: -1, toCall: 0 };
}

interface SeatViewWire {
  seat: number;
  playerId: string;
  stack: number;
  sittingOut: boolean;
  inHand: boolean;
}

/**
 * Folds one Submit/Tick/VoidHand event map into the view. Scans both the
 * broadcast stream and the recipient's own stream (hand_dealt is a privacy send,
 * so the button arrives on the self key). Returns the list of event types seen,
 * which the caller uses for turn-timer and fairness bookkeeping.
 */
export function updateView(view: MeshView, events: EventMap, selfId: string): string[] {
  const seen: string[] = [];
  for (const key of [BROADCAST, selfId]) {
    for (const env of events[key] ?? []) {
      seen.push(env.type);
      applyOne(view, env);
    }
  }
  return seen;
}

function applyOne(view: MeshView, env: Envelope): void {
  const data = (env.data ?? {}) as Record<string, unknown>;
  switch (env.type) {
    case "seat_update": {
      const seats = (data.seats as SeatViewWire[] | undefined) ?? [];
      const next = new Map<number, ViewSeat>();
      for (const s of seats) {
        next.set(s.seat, {
          playerId: s.playerId,
          stack: s.stack,
          sittingOut: s.sittingOut,
          inHand: s.inHand,
        });
      }
      view.seats = next;
      break;
    }
    case "hand_dealt": {
      view.handRunning = true;
      if (typeof data.buttonSeat === "number") view.buttonSeat = data.buttonSeat;
      view.toActSeat = -1;
      view.toCall = 0;
      break;
    }
    case "bet_placed": {
      if (typeof data.toAct === "number") view.toActSeat = data.toAct;
      if (typeof data.toCall === "number") view.toCall = data.toCall;
      break;
    }
    case "street_advanced": {
      // A new street reopens betting; the first actor is not announced by the
      // core, so the bot falls back to spamming check/call until one lands.
      view.toActSeat = -1;
      view.toCall = 0;
      break;
    }
    case "showdown": {
      view.handRunning = false;
      view.toActSeat = -1;
      view.toCall = 0;
      break;
    }
    default:
      break;
  }
}
