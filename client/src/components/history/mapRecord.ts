// Pure mapper: server hand-history records -> the shapes the client already
// knows how to render. Two targets:
//
//  1. `mapToHandRecord` produces exactly the `HandRecord` shape
//     `useGame(s => s.history)` holds (see src/store/gameStore.ts), so it is
//     replayer-ready the moment something pushes it into that store (see
//     README-style note in HistoryScreen.tsx for the wiring gap this leaves).
//  2. `mapToHoleCards` / `isMaskedHole` translate the API's ["??","??"]
//     masking sentinel into `Card | undefined` for the existing <PlayingCard>
//     component (undefined already means "face down" there).
//
// No I/O, no randomness, no mutation: every function here is a straight
// data-in/data-out transform, safe to unit test if/when a test runner exists.

import { Ev, type Card, type ServerEvent, type Street } from "@/net/protocol";
import type { HandRecord } from "@/store/gameStore";
import type { ApiAward, ApiEvent, ApiHandRecord } from "@/net/hands";

/** The server's masked-hole-cards sentinel (handsapi.maskHoleCards). */
const MASKED_HOLE = ["??", "??"];

export function isMaskedHole(hole: readonly string[]): boolean {
  return (
    hole.length === MASKED_HOLE.length &&
    hole[0] === MASKED_HOLE[0] &&
    hole[1] === MASKED_HOLE[1]
  );
}

/** Masked hole cards render as face-down (PlayingCard treats `undefined` as
 * face down); visible hole cards pass through as-is. */
export function mapToHoleCards(hole: readonly string[]): (Card | undefined)[] {
  if (isMaskedHole(hole)) return [undefined, undefined];
  return hole.map((c) => c);
}

function streetCardCount(street: string): number {
  switch (street.toLowerCase()) {
    case "flop":
      return 3;
    case "turn":
    case "river":
      return 1;
    default:
      return 0;
  }
}

/** One row of the per-seat showdown summary, independent of ShowdownResult's
 * "won" semantics (the API doesn't hand us a hand-class string per seat that
 * matches that interface, only Results[seatId] free text -- see the report
 * for why `handClass` doubles up as that whole description). */
function buildShowdownResults(api: ApiHandRecord) {
  const awardBySeat = new Map<number, number>();
  for (const award of api.Awards as ApiAward[]) {
    awardBySeat.set(award.SeatID, (awardBySeat.get(award.SeatID) ?? 0) + award.Amount);
  }
  return api.Seats.map((seat) => ({
    seat: seat.SeatID,
    hole: mapToHoleCards(seat.Hole).filter((c): c is Card => c !== undefined),
    handClass: api.Results[String(seat.SeatID)] ?? "",
    won: awardBySeat.get(seat.SeatID) ?? 0,
  }));
}

/** Builds the StreetAdvanced/Showdown event stream a replayer scrubs
 * through, mirroring how the server's history.ExportText walks Events (see
 * server/internal/history/export.go): each Kind=="street" event advances the
 * cumulative board by that street's known card count. */
function buildStreetEvents(api: ApiHandRecord): ServerEvent[] {
  const events: ServerEvent[] = [];
  let boardIdx = 0;
  let seq = 1;
  for (const ev of api.Events as ApiEvent[]) {
    if (ev.Kind !== "street") continue;
    const count = streetCardCount(ev.Street);
    boardIdx = Math.min(boardIdx + count, api.Board.length);
    events.push({
      type: Ev.StreetAdvanced,
      seq: seq++,
      data: {
        tableId: api.TableID,
        street: ev.Street as Street,
        board: api.Board.slice(0, boardIdx),
        pot: 0,
        nextToAct: -1,
      },
    });
  }
  return events;
}

/** Maps one full GET /api/hands/{id} record to the store's `HandRecord`
 * shape: a `handId` plus a `ServerEvent[]` timeline the existing
 * `HandReplayer` (via `boardAt`) can scrub through, in the same event
 * vocabulary the live table already produces.
 *
 * KNOWN GAP (see HistoryScreen.tsx / issue #24 report): the store's
 * `HandRecord[]` lives only in `useGame(s => s.history)`, populated by live
 * `hand_dealt`/`bet_placed`/... events over the socket. There is no store
 * action or HandReplayer prop that accepts an externally-fetched record, so
 * this function's output is not yet wireable into the *existing* replayer
 * without a small addition outside this module's ownership.
 */
export function mapToHandRecord(api: ApiHandRecord): HandRecord {
  const events: ServerEvent[] = [
    {
      type: Ev.HandDealt,
      seq: 0,
      data: {
        tableId: api.TableID,
        handId: api.HandID,
        commitment: api.Commitment,
        // The API doesn't tell us which seat belongs to the viewer once
        // already fetched as a full record (only maskHoleCards used that
        // context server-side); nothing downstream (boardAt) reads these
        // two fields today, so they're left honestly empty rather than
        // guessed.
        yourSeat: -1,
        yourHole: [],
        buttonSeat: api.ButtonSeat,
        blinds: api.Blinds,
      },
    },
    ...buildStreetEvents(api),
    {
      type: Ev.Showdown,
      seq: 1000,
      data: {
        tableId: api.TableID,
        handId: api.HandID,
        board: api.Board,
        results: buildShowdownResults(api),
        pots: [
          {
            amount: (api.Awards as ApiAward[]).reduce((sum, a) => sum + a.Amount, 0),
            winners: (api.Awards as ApiAward[]).map((a) => a.SeatID),
          },
        ],
      },
    },
    ...(api.SeedHex
      ? [
          {
            type: Ev.FairReveal,
            seq: 1001,
            data: { handId: api.HandID, commitment: api.Commitment, seed: api.SeedHex },
          } as ServerEvent,
        ]
      : []),
  ];

  return {
    handId: api.HandID,
    board: api.Board,
    events,
    commitment: api.Commitment,
    seed: api.SeedHex || undefined,
  };
}
