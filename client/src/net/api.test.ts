// @vitest-environment jsdom
//
// REST wrapper error-path tests. jsdom because the auth header path reads
// window.localStorage. fetch is mocked per test; no network is touched.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  AUTH_TOKEN_KEY,
  createRoom,
  getStoredToken,
  guestLogin,
  joinRoom,
  listSNG,
  listTables,
  quickseat,
  registerSNG,
} from "./api";
import { isOfflineMode, setOfflineMode } from "./mode";

type FetchArgs = [input: string | URL | Request, init?: RequestInit];

/** Minimal Response stand-in: request() only reads ok, status and json(). */
function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

function textResponse(status: number, text: string) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => JSON.parse(text) as unknown,
  };
}

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  window.localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  setOfflineMode(false);
});

describe("request error envelope handling", () => {
  it("throws an ApiError carrying status, code and message from the error envelope", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(402, { error: { code: "insufficient_funds", message: "not enough chips" } }),
    );
    const err = await registerSNG("sng-1").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.status).toBe(402);
    expect(apiErr.code).toBe("insufficient_funds");
    expect(apiErr.message).toBe("not enough chips");
    expect(apiErr.name).toBe("ApiError");
  });

  it("falls back to default code and message when the body is plain text", async () => {
    fetchMock.mockResolvedValue(textResponse(500, "internal server error"));
    const err = await listTables().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.status).toBe(500);
    expect(apiErr.code).toBe("unknown_error");
    expect(apiErr.message).toBe("request failed with status 500");
  });

  it("keeps defaults when the JSON body has no error envelope", async () => {
    fetchMock.mockResolvedValue(jsonResponse(404, { detail: "nope" }));
    const err = await joinRoom("ZZZZ").catch((e: unknown) => e);
    const apiErr = err as ApiError;
    expect(apiErr.code).toBe("unknown_error");
    expect(apiErr.message).toBe("request failed with status 404");
  });
});

describe("offline (nearby) mode guard", () => {
  it("throws loudly in dev before any fetch when offline mode is active", async () => {
    setOfflineMode(true);
    expect(isOfflineMode()).toBe(true);
    await expect(listTables()).rejects.toThrow(/blocked cloud call to \/api\/tables/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("allows cloud calls again after leaving offline mode", async () => {
    setOfflineMode(true);
    setOfflineMode(false);
    expect(isOfflineMode()).toBe(false);
    fetchMock.mockResolvedValue(jsonResponse(200, []));
    await expect(listTables()).resolves.toEqual([]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("request headers and payloads", () => {
  it("attaches the stored bearer token and JSON content type", async () => {
    window.localStorage.setItem(AUTH_TOKEN_KEY, "tok-123");
    fetchMock.mockResolvedValue(jsonResponse(200, []));
    await listSNG();
    const [url, init] = fetchMock.mock.calls[0] as FetchArgs;
    expect(String(url)).toMatch(/\/api\/sng$/);
    const headers = init?.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer tok-123");
    expect(headers.get("Content-Type")).toBe("application/json");
  });

  it("omits the Authorization header when no token is stored", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, []));
    await listTables();
    const [, init] = fetchMock.mock.calls[0] as FetchArgs;
    expect((init?.headers as Headers).get("Authorization")).toBeNull();
  });

  it("posts the room settings when creating a private room", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, { tableId: "t1", joinCode: "ABCD", joinUrl: "http://x/j/ABCD" }),
    );
    const req = { smallBlind: 5, bigBlind: 10, maxSeats: 6, visibility: "private" as const };
    const res = await createRoom(req);
    expect(res.joinCode).toBe("ABCD");
    const [url, init] = fetchMock.mock.calls[0] as FetchArgs;
    expect(String(url)).toMatch(/\/api\/rooms$/);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual(req);
  });

  it("posts the stake when quickseating", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { tableId: "t9" }));
    await expect(quickseat(5)).resolves.toEqual({ tableId: "t9" });
    const [, init] = fetchMock.mock.calls[0] as FetchArgs;
    expect(JSON.parse(String(init?.body))).toEqual({ smallBlind: 5 });
  });
});

describe("guest login", () => {
  it("persists the returned token to localStorage on success", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { token: "guest-tok", playerId: "p1" }));
    const res = await guestLogin();
    expect(res.playerId).toBe("p1");
    expect(getStoredToken()).toBe("guest-tok");
  });

  it("stores nothing when the login fails with a plain-text body", async () => {
    fetchMock.mockResolvedValue(textResponse(401, "unauthorized"));
    await expect(guestLogin()).rejects.toBeInstanceOf(ApiError);
    expect(getStoredToken()).toBeNull();
  });
});
