// Common peer-to-peer transport abstraction for offline LAN play (issue #28).
//
// The mesh speaks only JSON strings; every concrete transport implements the
// Connection interface below: an in-memory pipe for tests (this file), a WebRTC
// data channel (rtc.ts), and, on native, a local WebSocket reached over mDNS
// (the local-server-plugin). Because the interface is transport-blind, game
// screens and the cloud client can share it (see adapter note in the report).

/** One directed link to a single remote peer. All payloads are JSON strings. */
export interface Connection {
  /** Stable id of the REMOTE peer this link reaches. */
  readonly peerId: string;
  /** Enqueue one frame for delivery to the remote peer. */
  send(data: string): void;
  /** Register the single handler invoked for each inbound frame. */
  onMessage(cb: (data: string) => void): void;
  /** Register the handler invoked once when the link closes. */
  onClose(cb: () => void): void;
  /** Tear the link down; idempotent. */
  close(): void;
}

/**
 * One endpoint of an in-memory duplex link. A frame sent here is delivered to
 * the paired endpoint's message handler. Delivery can be blocked to simulate a
 * partition: while blocked, frames queue in order and flush on unblock. The
 * blocked flag lives on the RECEIVING endpoint (frames arriving at a peer are
 * what a partition withholds), so InMemoryNet.setBlocked targets the receiver.
 */
class MemEndpoint implements Connection {
  readonly peerId: string;
  private paired: MemEndpoint | null = null;
  private msgCb: ((d: string) => void) | null = null;
  private closeCb: (() => void) | null = null;
  private blocked = false;
  private queue: string[] = [];
  private closed = false;

  constructor(remotePeerId: string) {
    this.peerId = remotePeerId;
  }

  pair(other: MemEndpoint): void {
    this.paired = other;
  }

  send(data: string): void {
    if (this.closed || !this.paired) return;
    this.paired.receive(data);
  }

  private receive(data: string): void {
    if (this.closed) return;
    if (this.blocked) {
      this.queue.push(data);
      return;
    }
    this.msgCb?.(data);
  }

  /** Block or unblock inbound delivery to THIS endpoint; flush on unblock. */
  setBlocked(blocked: boolean): void {
    this.blocked = blocked;
    if (!blocked) {
      const pending = this.queue;
      this.queue = [];
      for (const m of pending) this.msgCb?.(m);
    }
  }

  onMessage(cb: (d: string) => void): void {
    this.msgCb = cb;
  }

  onClose(cb: () => void): void {
    this.closeCb = cb;
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.paired?.remoteClosed();
  }

  private remoteClosed(): void {
    this.closeCb?.();
  }
}

/**
 * A fully-connected in-memory network for deterministic mesh tests. Every pair
 * of peers gets a duplex link. Partitions and reconnects are simulated with
 * setBlocked / addPeer without any real timers, so a test drives time itself.
 */
export class InMemoryNet {
  // endpoints[owner][remote] is the Connection owner uses to reach remote.
  private readonly endpoints = new Map<string, Map<string, MemEndpoint>>();

  constructor(peerIds: string[]) {
    for (const id of peerIds) this.endpoints.set(id, new Map());
    for (let i = 0; i < peerIds.length; i++) {
      for (let j = i + 1; j < peerIds.length; j++) {
        this.linkPair(peerIds[i], peerIds[j]);
      }
    }
  }

  private linkPair(a: string, b: string): void {
    const ab = new MemEndpoint(b);
    const ba = new MemEndpoint(a);
    ab.pair(ba);
    ba.pair(ab);
    this.endpoints.get(a)!.set(b, ab);
    this.endpoints.get(b)!.set(a, ba);
  }

  /** Adds a late-joining peer, linking it to every existing peer. */
  addPeer(id: string): void {
    if (this.endpoints.has(id)) return;
    const existing = [...this.endpoints.keys()];
    this.endpoints.set(id, new Map());
    for (const other of existing) this.linkPair(id, other);
  }

  /** The connections `owner` uses to reach its peers. */
  connectionsFor(owner: string): Connection[] {
    return [...(this.endpoints.get(owner)?.values() ?? [])];
  }

  /** Withhold (or resume) delivery of frames sent from `from` to `to`. */
  setBlocked(from: string, to: string, blocked: boolean): void {
    this.endpoints.get(to)?.get(from)?.setBlocked(blocked);
  }

  /** Partition a peer from everyone (both directions). */
  isolate(peer: string, blocked: boolean): void {
    for (const other of this.endpoints.keys()) {
      if (other === peer) continue;
      this.setBlocked(peer, other, blocked);
      this.setBlocked(other, peer, blocked);
    }
  }
}
