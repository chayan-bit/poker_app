// Typed REST wrappers for auth, lobby, and hand-history endpoints. These
// mirror the shapes in server/internal/auth/handlers.go and
// server/internal/lobby/handlers.go exactly. No `any`.

const DEFAULT_API_URL = "http://localhost:8080";

function apiBase(): string {
  const fromEnv = (import.meta.env.VITE_API_URL as string | undefined)?.trim();
  return fromEnv && fromEnv.length > 0 ? fromEnv : DEFAULT_API_URL;
}

export const AUTH_TOKEN_KEY = "poker.authToken";

export function getStoredToken(): string | null {
  return window.localStorage.getItem(AUTH_TOKEN_KEY);
}

function storeToken(token: string): void {
  window.localStorage.setItem(AUTH_TOKEN_KEY, token);
}

/** Standard lobby error envelope: {"error":{"code","message"}}. */
export interface ApiErrorBody {
  error: { code: string; message: string };
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(
  path: string,
  init: RequestInit = {},
  opts: { auth?: boolean } = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  if (opts.auth !== false) {
    const token = getStoredToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }

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
      // Some endpoints (auth) return a plain text error body; keep defaults.
    }
    throw new ApiError(res.status, code, message);
  }
  return (await res.json()) as T;
}

// ---- Auth ----

export interface GuestLoginResponse {
  token: string;
  playerId: string;
}

/** POST /api/auth/guest, persists the returned token to localStorage. */
export async function guestLogin(): Promise<GuestLoginResponse> {
  const body = await request<GuestLoginResponse>(
    "/api/auth/guest",
    { method: "POST" },
    { auth: false },
  );
  storeToken(body.token);
  return body;
}

export interface UpgradeResponse {
  token: string;
  playerId: string;
  email: string;
}

/** POST /api/auth/upgrade: upgrades the current guest to a full account. */
export async function upgrade(email: string): Promise<UpgradeResponse> {
  const body = await request<UpgradeResponse>("/api/auth/upgrade", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
  storeToken(body.token);
  return body;
}

// ---- Lobby ----

export interface PublicTable {
  tableId: string;
  smallBlind: number;
  bigBlind: number;
  maxSeats: number;
}

/** GET /api/tables: the public lobby list. */
export function listTables(): Promise<PublicTable[]> {
  return request<PublicTable[]>("/api/tables", { method: "GET" });
}

export interface CreateRoomRequest {
  smallBlind: number;
  bigBlind: number;
  maxSeats: number;
  visibility: "private";
}

export interface CreateRoomResponse {
  tableId: string;
  joinCode: string;
  joinUrl: string;
}

/** POST /api/rooms: creates a private table and returns its join code. */
export function createRoom(
  req: CreateRoomRequest,
): Promise<CreateRoomResponse> {
  return request<CreateRoomResponse>("/api/rooms", {
    method: "POST",
    body: JSON.stringify(req),
  });
}

export interface JoinRoomResponse {
  tableId: string;
}

/** POST /api/rooms/join: resolves a private room's join code. */
export function joinRoom(code: string): Promise<JoinRoomResponse> {
  return request<JoinRoomResponse>("/api/rooms/join", {
    method: "POST",
    body: JSON.stringify({ code }),
  });
}

export interface QuickseatResponse {
  tableId: string;
}

/** POST /api/quickseat: joins (or creates) a public table at the given stake. */
export function quickseat(smallBlind: number): Promise<QuickseatResponse> {
  return request<QuickseatResponse>("/api/quickseat", {
    method: "POST",
    body: JSON.stringify({ smallBlind }),
  });
}

// ---- Hand history ----
// NOTE: server-side routes for these were not found under server/cmd/pokerd
// or server/internal/lobby at the time of writing (no "/api/hands" or
// "/api/players/me/hands" registration in server/cmd/pokerd/main.go). Wired
// here to the shapes described in the issue; will 404 against the current
// server until those routes are added server-side.

export interface HandHistoryEntry {
  handId: string;
  tableId: string;
  board: string[];
  playedAt: string;
}

/** GET /api/hands/{id}. */
export function getHand(id: string): Promise<HandHistoryEntry> {
  return request<HandHistoryEntry>(`/api/hands/${encodeURIComponent(id)}`, {
    method: "GET",
  });
}

/** GET /api/players/me/hands. */
export function myHands(): Promise<HandHistoryEntry[]> {
  return request<HandHistoryEntry[]>("/api/players/me/hands", {
    method: "GET",
  });
}
