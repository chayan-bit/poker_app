// Typed WebSocket client. One persistent socket. Responsibilities:
//   - envelope framing (v/type/seq/data)
//   - monotonic seq gap detection -> single `resync` command (no UI teardown)
//   - auto-reconnect with backoff; surfaces connection state for the top banner
//   - never runs game logic; only ships commands and dispatches typed events
//
// The store subscribes to onEvent / onStatus. The socket is intentionally the
// only place that touches WebSocket so the rest of the app stays testable and
// the mock server can be swapped in behind the same interface.

import {
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

const BACKOFF_MS = [250, 500, 1000, 2000, 4000] as const;

export class WsClient implements NetTransport {
  private ws: WebSocket | null = null;
  private lastSeq = 0;
  private attempt = 0;
  private closedByUser = false;
  private queue: Command[] = [];

  constructor(
    private readonly url: string,
    private readonly handlers: NetHandlers,
  ) {}

  connect(): void {
    this.closedByUser = false;
    this.open();
  }

  private open(): void {
    this.handlers.onStatus(this.attempt === 0 ? "connecting" : "reconnecting");
    const ws = new WebSocket(this.url);
    this.ws = ws;

    ws.onopen = () => {
      this.attempt = 0;
      this.handlers.onStatus("open");
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

  private scheduleReconnect(): void {
    this.handlers.onStatus("reconnecting");
    const delay = BACKOFF_MS[Math.min(this.attempt, BACKOFF_MS.length - 1)];
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
