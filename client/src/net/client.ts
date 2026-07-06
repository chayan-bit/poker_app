// Typed WebSocket client. One persistent socket. Responsibilities:
//   - envelope framing (v/type/seq/data)
//   - monotonic seq gap detection -> single `resync` command (no UI teardown)
//   - auto-reconnect with backoff; surfaces connection state for the top banner
//   - never runs game logic; only ships commands and dispatches typed events
//
// The store subscribes to onEvent / onStatus. The socket is intentionally the
// only place that touches WebSocket so the rest of the app stays testable and
// the mock server can be swapped in behind the same interface.
//
// Auth note: the server's Authenticator.FromRequest (server/internal/auth/
// auth.go ~L46-62) only reads an `Authorization: Bearer <token>` header or a
// signed `guest` cookie - it does not read a query-string token today. The
// browser WebSocket API cannot set custom headers on the handshake, so this
// client sends the token as a `?token=` query param (the conventional
// workaround). Until the server's ws.Gateway.Auth (server/internal/ws/
// gateway.go:76, `g.Auth(r)`) is updated to also check `r.URL.Query().Get
// ("token")`, real WS auth against the live server will fail with 401; this
// is a server-side gap, not fixed here (server code is out of scope).

import {
  Cmd,
  PROTOCOL_VERSION,
  type Command,
  type Envelope,
  type ServerEvent,
} from "./protocol";

export type ConnStatus = "connecting" | "open" | "reconnecting" | "closed";

export interface NetTransport {
  send(cmd: Command): void;
  close(): void;
}

export interface NetHandlers {
  onEvent(ev: ServerEvent): void;
  onStatus(status: ConnStatus): void;
  /** Called when a seq gap is detected so the client can request a resync. */
  onGap(lastGoodSeq: number): void;
}

export interface WsClientOptions {
  /** Auth token appended as `?token=`. Omit for unauthenticated dev sockets. */
  token?: string;
  /** If set, join_table + resync are sent automatically on every (re)open. */
  tableId?: string;
}

const BACKOFF_BASE_MS = 250;
const BACKOFF_CAP_MS = 5000;

/** Exponential backoff (base * 2^attempt) capped at BACKOFF_CAP_MS, with up to
 * 20% jitter so many clients reconnecting after an outage don't sync-thunder
 * the server. */
function backoffDelay(attempt: number): number {
  const raw = BACKOFF_BASE_MS * 2 ** attempt;
  const capped = Math.min(raw, BACKOFF_CAP_MS);
  const jitter = capped * 0.2 * Math.random();
  return Math.round(capped - jitter / 2 + jitter * Math.random());
}

export class WsClient implements NetTransport {
  private ws: WebSocket | null = null;
  private lastSeq = 0;
  private attempt = 0;
  private closedByUser = false;
  private queue: Command[] = [];
  private tableId: string | undefined;

  constructor(
    private readonly url: string,
    private readonly handlers: NetHandlers,
    private readonly opts: WsClientOptions = {},
  ) {
    this.tableId = opts.tableId;
  }

  connect(): void {
    this.closedByUser = false;
    this.open();
  }

  private buildUrl(): string {
    if (!this.opts.token) return this.url;
    const sep = this.url.includes("?") ? "&" : "?";
    return `${this.url}${sep}token=${encodeURIComponent(this.opts.token)}`;
  }

  private open(): void {
    this.handlers.onStatus(this.attempt === 0 ? "connecting" : "reconnecting");
    const ws = new WebSocket(this.buildUrl());
    this.ws = ws;

    ws.onopen = () => {
      this.attempt = 0;
      this.handlers.onStatus("open");
      // On (re)open, rejoin the table and request a fresh snapshot before
      // anything else, so a reconnect never leaves the client stuck on a
      // stale view.
      if (this.tableId) {
        this.rawSend({
          type: Cmd.JoinTable,
          data: { tableId: this.tableId },
        });
        this.rawSend({
          type: Cmd.Resync,
          data: { tableId: this.tableId, haveSeq: this.lastSeq },
        });
      }
      // Flush anything queued while offline.
      const pending = this.queue.splice(0);
      for (const cmd of pending) this.rawSend(cmd);
    };

    ws.onmessage = (msg) => this.handleFrame(msg.data);

    ws.onclose = () => {
      if (this.closedByUser) {
        this.handlers.onStatus("closed");
        return;
      }
      this.scheduleReconnect();
    };

    ws.onerror = () => {
      // onclose will follow; nothing to do here.
    };
  }

  /** Lets the store tell the client which table to rejoin/resync on reopen. */
  setTableId(tableId: string | undefined): void {
    this.tableId = tableId;
  }

  private scheduleReconnect(): void {
    this.handlers.onStatus("reconnecting");
    const delay = backoffDelay(this.attempt);
    this.attempt += 1;
    window.setTimeout(() => {
      if (!this.closedByUser) this.open();
    }, delay);
  }

  private handleFrame(raw: unknown): void {
    if (typeof raw !== "string") return;
    let env: Envelope;
    try {
      env = JSON.parse(raw) as Envelope;
    } catch {
      return; // ignore malformed frames rather than crash the UI
    }
    if (env.v !== PROTOCOL_VERSION) return;

    const ev = { type: env.type, seq: env.seq, data: env.data } as ServerEvent;

    // Errors and (re)snapshots don't participate in gap detection; a snapshot
    // is itself the recovery. Everything else must arrive in order.
    if (env.type === "table_snapshot") {
      this.lastSeq = env.seq ?? this.lastSeq;
      this.handlers.onEvent(ev);
      return;
    }
    if (env.type === "error") {
      this.handlers.onEvent(ev);
      return;
    }

    if (typeof env.seq === "number") {
      if (env.seq <= this.lastSeq) return; // duplicate / out of order, drop
      if (env.seq !== this.lastSeq + 1 && this.lastSeq !== 0) {
        // Gap: ask for a fresh snapshot, do NOT tear down the UI.
        this.handlers.onGap(this.lastSeq);
        return;
      }
      this.lastSeq = env.seq;
    }
    this.handlers.onEvent(ev);
  }

  private rawSend(cmd: Command): void {
    const env: Envelope = {
      v: PROTOCOL_VERSION,
      type: cmd.type,
      data: cmd.data,
    };
    this.ws?.send(JSON.stringify(env));
  }

  send(cmd: Command): void {
    if (cmd.type === Cmd.JoinTable) this.tableId = cmd.data.tableId;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.rawSend(cmd);
    } else {
      this.queue.push(cmd);
    }
  }

  /** Reset the applied-seq cursor, e.g. right before requesting a resync. */
  markResynced(seq: number): void {
    this.lastSeq = seq;
  }

  close(): void {
    this.closedByUser = true;
    this.ws?.close();
    this.ws = null;
  }
}
