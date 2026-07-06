// Typed REST wrappers for the read-only hand-history endpoints
// (server/internal/handsapi/handsapi.go), used exclusively by
// components/history/*.
//
//   GET /api/players/me/hands?limit=N -> ApiHandSummary[] (most-recent-first,
//     capped server-side at 100; default 20)
//   GET /api/hands/{id}               -> ApiHandRecord (hole cards masked as
//     ["??","??"] for seats the caller may not see)
//   GET /api/hands/{id}/text          -> plain-text PokerStars-style export
//
// This intentionally does not touch src/net/api.ts (owned by another change,
// and its existing getHand/myHands there predate the real server routes and
// no longer match their shapes -- see that file's own comment). It re-uses
// api.ts's already-public ApiError class and stored-token accessor, same
// pattern as src/net/social.ts.

import { ApiError, getStoredToken, type ApiErrorBody } from "./api";

const DEFAULT_API_URL = "http://localhost:8080";

function apiBase(): string {
  const fromEnv = (import.meta.env.VITE_API_URL as string | undefined)?.trim();
  return fromEnv && fromEnv.length > 0 ? fromEnv : DEFAULT_API_URL;
}

function authHeaders(): Headers {
  const headers = new Headers();
  const token = getStoredToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  return headers;
}

async function throwApiError(res: Response): Promise<never> {
  let code = "unknown_error";
  let message = `request failed with status ${res.status}`;
  try {
    const body = (await res.json()) as Partial<ApiErrorBody>;
    if (body.error) {
      code = body.error.code;
      message = body.error.message;
    }
  } catch {
    // Some failure modes return a plain-text body; keep the defaults.
  }
  throw new ApiError(res.status, code, message);
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${apiBase()}${path}`, { headers: authHeaders() });
  if (!res.ok) return throwApiError(res);
  return (await res.json()) as T;
}

async function getText(path: string): Promise<string> {
  const res = await fetch(`${apiBase()}${path}`, { headers: authHeaders() });
  if (!res.ok) return throwApiError(res);
  return await res.text();
}

/** One entry in GET /api/players/me/hands (server handsapi.handSummary). */
export interface ApiHandSummary {
  handId: string;
  tableId: string;
  startedAt: string; // RFC3339
  potWon: number;
}

/** Mirrors server/internal/history.SeatInfo, as serialized (no json tags ->
 * exported Go field names verbatim). Hole is ["??","??"] when masked. */
export interface ApiSeatInfo {
  SeatID: number;
  PlayerID: string;
  StartStack: number;
  Hole: string[];
}

/** Mirrors server/internal/history.Event. Kind=="street" marks a street
 * transition (see history.Recorder.OnStreet); other kinds are actions. */
export interface ApiEvent {
  Street: string;
  SeatID: number;
  Kind: string;
  Amount: number;
}

/** Mirrors server/internal/engine.Award. */
export interface ApiAward {
  SeatID: number;
  Amount: number;
}

/** Full GET /api/hands/{id} payload: history.HandRecord as serialized by
 * history.ExportJSON (plain json.Marshal, no struct tags). */
export interface ApiHandRecord {
  HandID: string;
  TableID: string;
  StartedAt: string; // RFC3339
  ButtonSeat: number;
  Blinds: [number, number];
  Commitment: string;
  SeedHex: string;
  Seats: ApiSeatInfo[];
  Events: ApiEvent[];
  Board: string[];
  Awards: ApiAward[];
  /** seatID (as string, since Go map[int]string keys serialize as strings) ->
   * human-readable result, e.g. "won 300 with Two Pair". */
  Results: Record<string, string>;
}

const DEFAULT_LIMIT = 20;

/** GET /api/players/me/hands?limit=N. limit is clamped to [1, 100] client
 * side too, matching the server's own defaulting/cap. */
export function fetchMyHands(limit: number = DEFAULT_LIMIT): Promise<ApiHandSummary[]> {
  const clamped = Math.max(1, Math.min(100, Math.trunc(limit) || DEFAULT_LIMIT));
  return getJSON<ApiHandSummary[]>(`/api/players/me/hands?limit=${clamped}`);
}

/** GET /api/hands/{id}. */
export function fetchHand(id: string): Promise<ApiHandRecord> {
  return getJSON<ApiHandRecord>(`/api/hands/${encodeURIComponent(id)}`);
}

/** GET /api/hands/{id}/text: the shareable plain-text export. */
export function fetchHandText(id: string): Promise<string> {
  return getText(`/api/hands/${encodeURIComponent(id)}/text`);
}
