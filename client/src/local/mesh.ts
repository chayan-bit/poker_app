// The replicated deterministic state machine each peer runs (issue #28).
//
// Every peer applies the same ordered log to its own #27 core, so replicating
// the log replicates the game. One peer per hand - the dealer button seat - is
// the coordinator: it assigns log seq numbers, broadcasts entries, runs the turn
// timer, and drives the fair-seed round. Every peer validates every entry
// against its own core (state-hash equality), so the coordinator holds no trust
// beyond ordering. Coordinator loss uses a deterministic successor
// (coordinator.ts), no election. Late joiners replay a snapshot.
//
// Determinism is sacred: no local clock or RNG reaches the core. Time enters
// only as coordinator-stamped entries (tick / fold_on_timeout); randomness only
// as the WebCrypto seed, itself a log entry.

import type { EventMap, LocalConfig } from "./core.ts";
import { coordinatorSeat, eligibleCount, type SeatInfo } from "./coordinator.ts";
import { FairSeedDriver } from "./meshseed.ts";
import {
  ACTION_RETRY_MS,
  DEFAULT_GRACE,
  DEFAULT_HEARTBEAT,
  DEFAULT_ROUND_TIMEOUT,
  DEFAULT_TURN_TIMEOUT,
  MAX_ACTION_TRIES,
  PROGRESS_EVENTS,
  type CoreLike,
  type MeshHooks,
  type MeshOptions,
} from "./meshtypes.ts";
import { updateView, newView, type MeshView } from "./meshview.ts";
import type { Connection } from "./transport.ts";
import type { ActionRequest, LogEntry, MeshMsg, Snapshot } from "./wire.ts";

/** One in-flight local action awaiting the coordinator's ack (retry bookkeeping). */
interface PendingAction {
  req: ActionRequest;
  sentMs: number;
  tries: number;
}

/** The stake-defining core fields every peer must share for hashes to agree. */
function sameCoreConfig(a: LocalConfig, b: LocalConfig): boolean {
  return (
    a.id === b.id &&
    a.smallBlind === b.smallBlind &&
    a.bigBlind === b.bigBlind &&
    (a.maxSeats ?? 9) === (b.maxSeats ?? 9)
  );
}

export class MeshNode {
  readonly selfId: string;
  private core: CoreLike;
  private readonly config: LocalConfig;
  private readonly clock: () => number;
  private readonly bootstrapId: string;
  /** Relay hub iff this peer bootstraps the mesh (the host in a star topology). */
  private readonly isHub: boolean;
  private readonly makeCore?: () => CoreLike;
  private readonly hooks: MeshHooks;
  private readonly heartbeatMs: number;
  private readonly graceMs: number;
  private readonly turnTimeoutMs: number;
  private readonly roundTimeoutMs: number;

  private conns = new Map<string, Connection>();
  private view: MeshView = newView();

  // Replicated log. log[seq-1] is the entry; head is the highest applied seq.
  private log: LogEntry[] = [];
  private head = 0;
  private buffer = new Map<number, LogEntry>();

  // Liveness (control-plane only): last heartbeat time per peer.
  private lastBeat = new Map<string, number>();
  private lastHeartbeatSent = 0;

  // Coordinator bookkeeping.
  private requestQueue: ActionRequest[] = [];
  private lastActionMs = 0;
  private dealingInProgress = false;
  /** reqIds this coordinator has already sequenced, so retries never double-apply. */
  private readonly processedReqs = new Set<string>();

  // Local-action delivery: pending requests awaiting an ack, and a reqId counter.
  private readonly pending = new Map<string, PendingAction>();
  private reqSeq = 0;

  /** True while rebuilding from a snapshot after rejecting a divergent entry. */
  private resyncing = false;

  // Fair-seed round machine (coordinator side) + verification (every peer).
  private readonly fair: FairSeedDriver;

  constructor(opts: MeshOptions) {
    this.selfId = opts.selfId;
    this.core = opts.core;
    this.config = opts.config;
    this.clock = opts.clock;
    this.bootstrapId = opts.bootstrapId;
    this.isHub = opts.selfId === opts.bootstrapId;
    this.makeCore = opts.makeCore;
    this.hooks = opts.hooks ?? {};
    this.heartbeatMs = opts.heartbeatMs ?? DEFAULT_HEARTBEAT;
    this.graceMs = opts.graceMs ?? DEFAULT_GRACE;
    this.turnTimeoutMs = opts.turnTimeoutMs ?? opts.config.turnTimeoutMs ?? DEFAULT_TURN_TIMEOUT;
    this.roundTimeoutMs = opts.roundTimeoutMs ?? DEFAULT_ROUND_TIMEOUT;
    this.fair = new FairSeedDriver({
      selfId: this.selfId,
      roundTimeoutMs: this.roundTimeoutMs,
      broadcast: (m) => this.broadcast(m),
      sendTo: (p, m) => this.sendTo(p, m),
      onDishonest: (s) => this.hooks.onDishonestDealer?.(s),
      onSeed: (s) => this.onSeedReady(s),
    });
    for (const c of opts.connections) this.attach(c);
  }

  /** The finalized fair seed: log it and nudge the core to deal the hand. */
  private onSeedReady(seedHex: string): void {
    this.dealingInProgress = true;
    this.emit({ kind: "seed", actor: this.selfId, seedHex });
    // sit_in on our own seat triggers startHandIfReady, dealing the hand.
    this.emit({ kind: "submit", actor: this.selfId, envelope: { v: 1, type: "sit_in" } });
  }

  /** Registers a connection and wires its inbound handler. */
  attach(conn: Connection): void {
    this.conns.set(conn.peerId, conn);
    conn.onMessage((data) => this.onFrame(data));
  }


  /**
   * A local player's chosen action; routed to the current coordinator. When the
   * coordinator is a remote peer the request is tracked and retried until acked
   * (or surfaced as undelivered), so a momentarily-not-open channel or a lost
   * frame does not silently drop the action (issue #28 hardening).
   */
  submitLocalAction(envelope: object): void {
    const coord = this.coordinatorPeerId();
    if (coord === this.selfId) {
      this.requestQueue.push({ actor: this.selfId, envelope: envelope as ActionRequest["envelope"] });
      return;
    }
    const reqId = `${this.selfId}:${(this.reqSeq += 1)}`;
    const req: ActionRequest = { actor: this.selfId, envelope: envelope as ActionRequest["envelope"], reqId };
    this.pending.set(reqId, { req, sentMs: this.clock(), tries: 1 });
    this.sendTo(coord, { t: "request", from: this.selfId, req });
  }

  /** Resend unacked action requests to the (possibly rotated) coordinator. */
  private retryPending(now: number): void {
    for (const [reqId, p] of this.pending) {
      if (now - p.sentMs < ACTION_RETRY_MS) continue;
      if (p.tries >= MAX_ACTION_TRIES) {
        this.pending.delete(reqId);
        this.hooks.onActionUndelivered?.(p.req);
        continue;
      }
      const coord = this.coordinatorPeerId();
      if (coord === this.selfId) {
        // We became the coordinator: sequence it ourselves and stop retrying.
        this.pending.delete(reqId);
        this.requestQueue.push(p.req);
        continue;
      }
      p.tries += 1;
      p.sentMs = now;
      this.sendTo(coord, { t: "request", from: this.selfId, req: p.req });
    }
  }

  /**
   * One control beat (host interval, or a test's virtual clock): emit
   * heartbeats, drain gossip, and run coordinator duties when we hold the role.
   */
  tick(nowMs?: number): void {
    const now = nowMs ?? this.clock();
    this.maybeHeartbeat(now);
    this.drainBuffer();
    this.retryPending(now);
    if (this.isCoordinator()) this.coordinatorDuties(now);
    // Drop any half-run fair round if we are no longer the coordinator, so a
    // later coordination stint never resurrects a stale seed for a new hand.
    else this.fair.abandon();
  }

  /** A snapshot for a late joiner: full log plus the authoritative hash. */
  snapshot(): Snapshot {
    return {
      config: this.config,
      entries: this.log.slice(0, this.head),
      head: this.head,
      stateHash: this.core.stateHash(),
    };
  }

  /**
   * Adopts a peer's snapshot by replaying every entry into this fresh core and
   * checking the result hash matches; a second snapshot, when supplied, is
   * cross-checked so the joiner never trusts a single peer.
   */
  adoptSnapshot(primary: Snapshot, secondary?: Snapshot): void {
    if (!sameCoreConfig(this.config, primary.config)) {
      throw new Error("mesh: snapshot config disagrees with local core config; refusing to adopt");
    }
    if (secondary && secondary.stateHash !== primary.stateHash) {
      throw new Error("mesh: snapshot hashes disagree; refusing to trust either");
    }
    this.log = [];
    this.head = 0;
    this.view = newView();
    for (const entry of primary.entries) {
      this.runOnCore(entry);
      this.log[entry.seq - 1] = entry;
      this.head = entry.seq;
      this.projectView(entry, this.lastEvents);
    }
    const got = this.core.stateHash();
    if (got !== primary.stateHash) {
      throw new Error(`mesh: replay hash ${got.slice(0, 12)} != snapshot ${primary.stateHash.slice(0, 12)}`);
    }
  }

  headSeq(): number {
    return this.head;
  }

  stateHash(): string {
    return this.core.stateHash();
  }

  getView(): MeshView {
    return this.view;
  }


  private seatsForCoord(): Map<number, SeatInfo> {
    return this.view.seats as Map<number, SeatInfo>;
  }

  private aliveSet(now = this.clock()): Set<string> {
    const alive = new Set<string>([this.selfId]);
    for (const [peer, at] of this.lastBeat) {
      if (now - at <= this.graceMs) alive.add(peer);
    }
    return alive;
  }

  private currentCoordSeat(): number {
    return coordinatorSeat(this.seatsForCoord(), this.aliveSet(), this.view.buttonSeat, this.view.handRunning);
  }

  /** Peer id of the coordinator; the bootstrap peer until a seat is occupied. */
  coordinatorPeerId(): string {
    const seat = this.currentCoordSeat();
    if (seat < 0) return this.bootstrapId;
    return this.view.seats.get(seat)!.playerId;
  }

  isCoordinator(): boolean {
    return this.coordinatorPeerId() === this.selfId && this.canCoordinate();
  }

  /**
   * Split-brain guard: only coordinate if at least one OTHER peer is visible. A
   * peer partitioned alone never sequences divergent hands (clean catch-up on
   * heal); two genuine survivors still see each other so play continues. A
   * symmetric N/N split is out of scope for LAN friend play (issue #28).
   */
  private canCoordinate(): boolean {
    return this.aliveSet().size >= 2;
  }


  private coordinatorDuties(now: number): void {
    // 1. Sequence any queued action requests.
    while (this.requestQueue.length > 0) {
      const req = this.requestQueue.shift()!;
      this.emit({ kind: "submit", actor: req.actor, envelope: req.envelope });
    }

    // 2. Mid-hand dealer loss: coordinator while a hand runs but not the button
    //    seat means the dealer dropped. Void and continue.
    if (this.view.handRunning && this.currentCoordSeat() !== this.view.buttonSeat) {
      this.emit({ kind: "void", actor: this.selfId });
      this.dealingInProgress = false;
      this.fair.abandon();
      return;
    }

    // 3. Turn timer: stamp fold_on_timeout when the actor has stalled.
    if (this.view.handRunning && now - this.lastActionMs >= this.turnTimeoutMs) {
      this.emit({
        kind: "submit",
        actor: "",
        envelope: { v: 1, type: "fold_on_timeout", data: { tableId: this.config.id, nowMs: now } },
        nowMs: now,
      });
      return;
    }

    // 4. Between hands: run the fair-seed round, then deal.
    if (!this.view.handRunning && !this.dealingInProgress) {
      if (eligibleCount(this.seatsForCoord(), this.aliveSet()) >= 2) {
        this.fair.stepCoordinator(now, this.eligibleAlivePlayers());
      }
    }
  }

  private eligibleAlivePlayers(): string[] {
    const alive = this.aliveSet();
    const out: string[] = [];
    for (const info of this.view.seats.values()) {
      if (alive.has(info.playerId) && info.stack > 0 && !info.sittingOut) out.push(info.playerId);
    }
    return out.sort();
  }


  private lastEvents: EventMap = {};

  private emit(partial: Omit<LogEntry, "seq" | "hashAfter" | "by">): void {
    const seq = this.head + 1;
    const entry: LogEntry = { ...partial, seq, hashAfter: "", by: this.selfId };
    this.runOnCore(entry);
    entry.hashAfter = this.core.stateHash();
    this.log[seq - 1] = entry;
    this.head = seq;
    this.projectView(entry, this.lastEvents);
    this.broadcast({ t: "entries", from: this.selfId, entries: [entry] });
  }

  private ingest(entry: LogEntry): void {
    if (this.resyncing) return; // a snapshot will supersede everything in flight
    if (entry.seq <= this.head) return; // duplicate
    if (entry.seq !== this.head + 1) {
      this.buffer.set(entry.seq, entry);
      return;
    }
    this.apply(entry);
    this.drainBuffer();
  }

  private drainBuffer(): void {
    if (this.resyncing) return;
    let next = this.buffer.get(this.head + 1);
    while (next && !this.resyncing) {
      this.buffer.delete(next.seq);
      this.apply(next);
      next = this.buffer.get(this.head + 1);
    }
  }

  /**
   * A sequencer is authorized iff it is the bootstrap peer (pre-seating) or it
   * currently occupies a seat. This is derived purely from the replicated seat
   * view (not the racy liveness set), so it never false-rejects during a
   * coordinator handoff; it blocks any non-participant from injecting entries.
   * Seed biasing by a seated peer is separately caught by each participant's
   * own-share verification in meshseed.ts.
   */
  private isAuthorizedSequencer(by: string): boolean {
    if (by === this.bootstrapId) return true;
    for (const info of this.view.seats.values()) if (info.playerId === by) return true;
    return false;
  }

  private apply(entry: LogEntry): void {
    // Authenticate the sequencer before mutating the core.
    if (!this.isAuthorizedSequencer(entry.by)) {
      this.beginResync("unauthorized");
      return;
    }
    this.runOnCore(entry);
    const localHash = this.core.stateHash();
    if (entry.hashAfter && localHash !== entry.hashAfter) {
      // Divergent: the entry does NOT reproduce the coordinator's hash. Never
      // commit it (that would fork the replicated state); flag it and rebuild
      // from a peer snapshot instead. Dishonest-dealer detection (combine(shares)
      // === logged seed) lives in the async fair "seed" handler (meshseed.ts).
      this.hooks.onDivergence?.(entry, localHash);
      this.beginResync("divergence");
      return;
    }
    this.log[entry.seq - 1] = entry;
    this.head = entry.seq;
    this.projectView(entry, this.lastEvents);
  }

  /**
   * Rejects the in-flight divergent/unauthorized state and asks peers for a
   * fresh snapshot to rebuild from. The WASM core cannot undo a bad mutation, so
   * recovery rebuilds a clean core (when a factory is supplied) and replays the
   * adopted snapshot into it. Idempotent while a resync is already pending.
   */
  private beginResync(reason: "divergence" | "unauthorized"): void {
    if (this.resyncing) return;
    this.resyncing = true;
    this.buffer.clear();
    this.hooks.onResync?.(reason);
    this.broadcast({ t: "snapshot_req", from: this.selfId });
  }

  private adoptForResync(snap: Snapshot): void {
    try {
      if (this.makeCore) this.core = this.makeCore();
      this.resyncing = false; // adoptSnapshot replays cleanly into the fresh core
      this.adoptSnapshot(snap);
      this.hooks.onResync?.("recovered");
    } catch {
      this.resyncing = true; // stay in resync; another snapshot may arrive
    }
  }

  private runOnCore(entry: LogEntry): void {
    switch (entry.kind) {
      case "submit":
        this.lastEvents = this.core.submit(entry.actor, JSON.stringify(entry.envelope));
        break;
      case "seed":
        this.core.setSeed(entry.seedHex ?? "");
        this.lastEvents = {};
        break;
      case "tick":
        this.lastEvents = this.core.tick(entry.nowMs ?? 0);
        break;
      case "void":
        this.lastEvents = this.core.voidHand();
        break;
    }
  }

  private projectView(entry: LogEntry, events: EventMap): void {
    const seen = updateView(this.view, events, this.selfId);
    if (entry.kind === "void") this.view.handRunning = false;
    if (seen.includes("hand_dealt")) this.dealingInProgress = false;
    // Reset the turn timer only on real progress, so no-op out-of-turn errors do
    // not keep a departed player's turn from ever timing out.
    if (this.view.handRunning && PROGRESS_EVENTS.some((t) => seen.includes(t))) {
      this.lastActionMs = this.clock();
    }
    this.hooks.onApplied?.(entry, events, this.view);
  }

  /** Number of hands dealt so far (one "seed" entry precedes each deal). */
  dealtHands(): number {
    let n = 0;
    for (let i = 0; i < this.head; i++) if (this.log[i]?.kind === "seed") n++;
    return n;
  }


  /**
   * Routes one inbound frame. Directed frames (`to` set) not addressed to us are
   * forwarded when we are the relay hub (the host); broadcast frames (no `to`)
   * are handled locally and, on the hub, fanned out to every other peer. This is
   * what lets a rotated guest-coordinator's messages reach a guest it has no
   * direct WebRTC link to in a star topology (issue #28 fix): the propagation
   * path is guest-coordinator -> host hub -> other guest, for both the fair-seed
   * round and the entries broadcast, so the replicated log converges on every
   * peer across full button rotation.
   */
  private onFrame(data: string): void {
    let msg: MeshMsg;
    try {
      msg = JSON.parse(data) as MeshMsg;
    } catch {
      return;
    }
    if (msg.to !== undefined && msg.to !== this.selfId) {
      if (this.isHub) this.conns.get(msg.to)?.send(data);
      return;
    }
    if (msg.to === undefined && this.isHub) this.relayBroadcast(data, msg.from);
    this.handle(msg);
  }

  /** Hub fan-out: resend a guest's broadcast to every peer but its origin. */
  private relayBroadcast(data: string, from: string): void {
    for (const [peerId, conn] of this.conns) {
      if (peerId !== from) conn.send(data);
    }
  }

  private handle(msg: MeshMsg): void {
    switch (msg.t) {
      case "hello":
      case "heartbeat":
        this.lastBeat.set(msg.from, this.clock());
        if (msg.t === "heartbeat" && msg.head > this.head) {
          this.sendTo(msg.from, { t: "need", from: this.selfId, have: this.head });
        }
        break;
      case "need":
        this.serveNeed(msg.from, msg.have);
        break;
      case "entries":
        // Live (non-gossip) frames must come from an authorized sequencer; gossip
        // catch-up (serveNeed) is exempt from the origin check. Either way each
        // entry is re-authorized by `by` and hash-validated on apply.
        if (msg.gossip !== true && !this.isAuthorizedSequencer(msg.from)) {
          this.beginResync("unauthorized");
          break;
        }
        for (const e of msg.entries) this.ingest(e);
        break;
      case "request":
        this.onRequest(msg.from, msg.req);
        break;
      case "ack":
        this.pending.delete(msg.reqId);
        break;
      case "snapshot_req":
        // A resyncing node's own state is suspect; do not serve snapshots from it.
        if (!this.resyncing) this.sendTo(msg.from, { t: "snapshot", from: this.selfId, snap: this.snapshot() });
        break;
      case "snapshot":
        if (this.resyncing) this.adoptForResync(msg.snap);
        break;
      case "fair":
        void this.fair.handle(msg, this.isCoordinator());
        break;
    }
  }

  /** Coordinator side of an action request: sequence once, ack every retry. */
  private onRequest(from: string, req: ActionRequest): void {
    if (this.coordinatorPeerId() !== this.selfId) return;
    if (!req.reqId || !this.processedReqs.has(req.reqId)) {
      if (req.reqId) this.processedReqs.add(req.reqId);
      // Queue by coordinator identity; the quorum guard only gates emission,
      // so bootstrap proposals are not lost before quorum forms.
      this.requestQueue.push(req);
    }
    if (req.reqId) this.sendTo(from, { t: "ack", from: this.selfId, reqId: req.reqId });
  }

  private serveNeed(peer: string, have: number): void {
    if (this.resyncing) return;
    if (have >= this.head) return;
    const entries = this.log.slice(have, this.head);
    if (entries.length > 0) this.sendTo(peer, { t: "entries", from: this.selfId, entries, gossip: true });
  }

  private maybeHeartbeat(now: number): void {
    if (now - this.lastHeartbeatSent < this.heartbeatMs) return;
    this.lastHeartbeatSent = now;
    this.broadcast({
      t: "heartbeat", from: this.selfId, head: this.head, nowMs: now, coordSeat: this.currentCoordSeat(),
    });
  }

  /** Fan a frame to every direct link with no `to`, so the hub relays it on. */
  private broadcast(msg: MeshMsg): void {
    const data = JSON.stringify(msg);
    for (const conn of this.conns.values()) conn.send(data);
  }

  /**
   * Send a directed frame to `peer`. If we have no direct link (a guest reaching
   * a non-adjacent guest in a star), route it through the relay hub, which
   * forwards it the last hop. Stamping `to` is what tells the hub to forward
   * rather than treat the frame as its own.
   */
  private sendTo(peer: string, msg: MeshMsg): void {
    const framed = JSON.stringify({ ...msg, to: peer });
    const direct = this.conns.get(peer);
    if (direct) {
      direct.send(framed);
      return;
    }
    this.conns.get(this.bootstrapId)?.send(framed);
  }
}
