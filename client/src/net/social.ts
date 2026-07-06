// Typed REST wrappers for the friends/presence endpoints. These mirror the
// shapes in server/internal/social/handlers.go and presence.go exactly:
//   - GET  /api/friends              -> FriendEntry[] (id, name, presence)
//   - GET  /api/friends/pending      -> PendingEntry[] (incoming requests)
//   - POST /api/friends/request      {playerId}
//   - POST /api/friends/accept       {playerId} (playerId = original sender)
//   - POST /api/friends/decline      {playerId} (playerId = original sender)
//   - POST /api/friends/remove       {playerId}
//   - GET  /api/friends/{id}/table   -> {tableId} | 403 not_friends | 404 not_at_table
//
// This file intentionally does not touch src/net/api.ts (owned by another
// change): it re-uses that module's exported ApiError class and stored-token
// accessor (both already public exports) but defines its own tiny fetch
// wrapper rather than reaching into api.ts's private `request` helper.

import { ApiError, getStoredToken, type ApiErrorBody } from "./api";

const DEFAULT_API_URL = "http://localhost:8080";

function apiBase(): string {
  const fromEnv = (import.meta.env.VITE_API_URL as string | undefined)?.trim();
  return fromEnv && fromEnv.length > 0 ? fromEnv : DEFAULT_API_URL;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  const token = getStoredToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(`${apiBase()}${path}`, { ...init, headers });
  if (!res.ok) {
    let code = "unknown_error";
    let message = `request failed with status ${res.status}`;
    try {
      const body = (await res.json()) as Partial<ApiErrorBody>;
      if (body.error) {
        code = body.error.code;
        message = body.error.message;
      }
    } catch {
      // some endpoints may return a plain-text error body; keep the defaults
    }
    throw new ApiError(res.status, code, message);
  }
  return (await res.json()) as T;
}

// ---- Presence ----

/** Mirrors server/internal/social/presence.go's Status: state is
 * "offline" | "lobby" | "table"; tableId is only populated for "table". */
export interface PresenceStatus {
  state: "offline" | "lobby" | "table";
  tableId: string;
}

// ---- Friends ----

export interface FriendEntry {
  playerId: string;
  name?: string;
  status: PresenceStatus;
}

export interface PendingEntry {
  playerId: string;
  name?: string;
}

/** GET /api/friends: the caller's confirmed friends, each with live presence. */
export function listFriends(): Promise<FriendEntry[]> {
  return request<FriendEntry[]>("/api/friends", { method: "GET" });
}

/** GET /api/friends/pending: incoming friend requests awaiting a decision. */
export function listPendingFriends(): Promise<PendingEntry[]> {
  return request<PendingEntry[]>("/api/friends/pending", { method: "GET" });
}

/** POST /api/friends/request {playerId}: send a friend request. */
export function requestFriend(playerId: string): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>("/api/friends/request", {
    method: "POST",
    body: JSON.stringify({ playerId }),
  });
}

/** POST /api/friends/accept {playerId}: playerId is the original sender. */
export function acceptFriend(playerId: string): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>("/api/friends/accept", {
    method: "POST",
    body: JSON.stringify({ playerId }),
  });
}

/** POST /api/friends/decline {playerId}: playerId is the original sender. */
export function declineFriend(playerId: string): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>("/api/friends/decline", {
    method: "POST",
    body: JSON.stringify({ playerId }),
  });
}

/** POST /api/friends/remove {playerId}. */
export function removeFriend(playerId: string): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>("/api/friends/remove", {
    method: "POST",
    body: JSON.stringify({ playerId }),
  });
}

export interface FriendTableResponse {
  tableId: string;
}

/** GET /api/friends/{id}/table: the friend's current table, for railing.
 * Throws ApiError(403, "not_friends") or ApiError(404, "not_at_table"). */
export function getFriendTable(playerId: string): Promise<FriendTableResponse> {
  return request<FriendTableResponse>(
    `/api/friends/${encodeURIComponent(playerId)}/table`,
    { method: "GET" },
  );
}
