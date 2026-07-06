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
  DEFAULT_GRACE,
  DEFAULT_HEARTBEAT,
  DEFAULT_ROUND_TIMEOUT,
  DEFAULT_TURN_TIMEOUT,
  PROGRESS_EVENTS,
  type CoreLike,
  type MeshHooks,
  type MeshOptions,
} from "./meshtypes.ts";
import { updateView, newView, type MeshView } from "./meshview.ts";
import type { Connection } from "./transport.ts";
import type { ActionRequest, LogEntry, MeshMsg, Snapshot } from "./wire.ts";

export class MeshNode {
  readonly selfId: string;
  private readonly core: CoreLike;
  private readonly config: LocalConfig;
  private readonly clock: () => number;
  private readonly bootstrapId: string;
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

  // Fair-seed round machine (coordinator side) + verification (every peer).
  private readonly fair: FairSeedDriver;

  constructor(opts: MeshOptions) {
    this.selfId = opts.selfId;
    this.core = opts.core;
    this.config = opts.config;
    this.clock = opts.clock;
    this.bootstrapId = opts.bootstrapId;
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


  /** A local player's chosen action; routed to the current coordinator. */
  submitLocalAction(envelope: object): void {
    const req: ActionRequest = { actor: this.selfId, envelope: envelope as ActionRequest["envelope"] };
    const coord = this.coordinatorPeerId();
    if (coord === this.selfId) this.requestQueue.push(req);
    else this.sendTo(coord, { t: "request", from: this.selfId, req });
  }

  /**
   * One control beat (host interval, or a test's virtual clock): emit
   * heartbeats, drain gossip, and run coordinator duties when we hold the role.
   */
  tick(nowMs?: number): void {
    const now = nowMs ?? this.clock();
    this.maybeHeartbeat(now);
    this.drainBuffer();
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
    if (entry.seq <= this.head) return; // duplicate
    if (entry.seq !== this.head + 1) {
      this.buffer.set(entry.seq, entry);
      return;
    }
    this.apply(entry);
    this.drainBuffer();
  }

  private drainBuffer(): void {
    let next = this.buffer.get(this.head + 1);
    while (next) {
      this.buffer.delete(next.seq);
      this.apply(next);
      next = this.buffer.get(this.head + 1);
    }
  }

  private apply(entry: LogEntry): void {
    this.runOnCore(entry);
    const localHash = this.core.stateHash();
    if (entry.hashAfter && localHash !== entry.hashAfter) {
      this.hooks.onDivergence?.(entry, localHash);
    }
    // Dishonest-dealer detection (combine(shares) === logged seed) lives in the
    // async fair "seed" handler (meshseed.ts), not here, so entry-before-message
    // ordering cannot raise a false positive.
    this.log[entry.seq - 1] = entry;
    this.head = entry.seq;
    this.projectView(entry, this.lastEvents);
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


  private onFrame(data: string): void {
    let msg: MeshMsg;
    try {
      msg = JSON.parse(data) as MeshMsg;
    } catch {
      return;
    }
    this.handle(msg);
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
        for (const e of msg.entries) this.ingest(e);
        break;
      case "request":
        // Queue by coordinator identity; the quorum guard only gates emission,
        // so bootstrap proposals are not lost before quorum forms.
        if (this.coordinatorPeerId() === this.selfId) this.requestQueue.push(msg.req);
        break;
      case "snapshot_req":
        this.sendTo(msg.from, { t: "snapshot", from: this.selfId, snap: this.snapshot() });
        break;
      case "snapshot":
        break; // adoption is driven explicitly via adoptSnapshot
      case "fair":
        void this.fair.handle(msg, this.isCoordinator());
        break;
    }
  }

  private serveNeed(peer: string, have: number): void {
    if (have >= this.head) return;
    const entries = this.log.slice(have, this.head);
    if (entries.length > 0) this.sendTo(peer, { t: "entries", from: this.selfId, entries });
  }

  private maybeHeartbeat(now: number): void {
    if (now - this.lastHeartbeatSent < this.heartbeatMs) return;
    this.lastHeartbeatSent = now;
    this.broadcast({
      t: "heartbeat", from: this.selfId, head: this.head, nowMs: now, coordSeat: this.currentCoordSeat(),
    });
  }

  private broadcast(msg: MeshMsg): void {
    const data = JSON.stringify(msg);
    for (const conn of this.conns.values()) conn.send(data);
  }

  private sendTo(peer: string, msg: MeshMsg): void {
    this.conns.get(peer)?.send(JSON.stringify(msg));
  }
}
