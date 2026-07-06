// The nearby (offline P2P) session controller. It assembles the real #27 table
// core, the #28 replicated mesh, and the store bridge into one live session, and
// drives the serverless WebRTC signaling. It is the single owner of the mesh
// node lifecycle and the only writer of nearby UI state.
//
// Assembly mirrors simulation.mts, but with real WebRTC links instead of the
// in-memory net and a wall clock instead of a virtual one. The host is the
// bootstrap peer, first seat, AND the relay hub: guests connect only to the host
// (a star) and reach one another through the host's relay in the mesh layer. The
// host's table config travels inside the invite blob so every guest builds a
// byte-identical core from the host's stakes/stack rather than a local default.

import { LocalCore, initLocalCore, type LocalConfig } from "@/local/core";
import { MeshNode } from "@/local/mesh";
import { MeshBridge } from "@/local/storeBridge";
import { acceptOffer, createOffer, type RtcOptions, type RtcState } from "@/local/rtc";
import type { Connection } from "@/local/transport";
import { useGame } from "@/store/gameStore";
import { displayName, encodePeerId } from "./names";
import { useNearby, type ConnectionState, type NearbyConfig, type SummaryRow } from "./nearbyStore";

const TICK_MS = 250;
const TURN_TIMEOUT_MS = 30_000;
const ROUND_TIMEOUT_MS = 4_000;
const GRACE_MS = 6_000;
const RECONNECT_TOAST_MS = 5_000;
/** How long a seat is held after this peer is left alone, awaiting a reconnect. */
const RECONNECT_GRACE_MS = 30_000;
const MAX_SEATS = 9;
const SEAT_CONFIRM_MS = 3_000;
const NOTICE_MS = 4_000;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function meshConfig(c: NearbyConfig): LocalConfig {
  return {
    id: "nearby",
    maxSeats: MAX_SEATS,
    smallBlind: c.smallBlind,
    bigBlind: c.bigBlind,
    autoStart: true,
    turnTimeoutMs: TURN_TIMEOUT_MS,
    disconnectGraceMs: GRACE_MS,
  };
}

/** Maps a raw RTC link state onto the coarse UI connection indicator. */
const CONN_STATE: Record<RtcState, ConnectionState> = {
  connecting: "connecting",
  connected: "connected",
  disconnected: "unstable",
  failed: "lost",
  closed: "lost",
};

/** An in-progress host invite: the offer blob to share and a way to accept the
 *  scanned/pasted answer that comes back. */
export interface HostInvite {
  offerBlob: string;
  accept: (answerBlob: string) => Promise<void>;
}

export class NearbySession {
  readonly selfId: string;
  private readonly cfg: NearbyConfig;
  private readonly config: LocalConfig;
  private readonly isHost: boolean;
  private readonly node: MeshNode;
  private readonly bridge: MeshBridge;
  private readonly conns: Connection[] = [];
  private readonly buyIns = new Map<string, number>();
  private readonly closedPeers = new Set<string>();
  private timer: ReturnType<typeof setInterval> | null = null;
  private endTimer: ReturnType<typeof setTimeout> | null = null;
  private unsub: (() => void) | null = null;
  private handsPlayed = 0;
  private biggestPot = 0;
  private ended = false;

  private constructor(selfId: string, cfg: NearbyConfig, bootstrapId: string) {
    this.selfId = selfId;
    this.cfg = cfg;
    this.isHost = selfId === bootstrapId;
    this.config = meshConfig(cfg);
    const core = new LocalCore(this.config, "");
    this.bridge = new MeshBridge({
      selfId,
      config: this.config,
      nameFor: displayName,
      submit: (env) => this.node.submitLocalAction(env),
      onVoid: () => this.onVoid(),
    });
    this.node = new MeshNode({
      selfId,
      core,
      config: this.config,
      connections: [],
      clock: () => Date.now(),
      bootstrapId,
      // Resync rebuilds a clean core from a peer snapshot when a divergent entry
      // is rejected; the WASM core cannot roll back a bad mutation in place.
      makeCore: () => new LocalCore(this.config, ""),
      graceMs: GRACE_MS,
      turnTimeoutMs: TURN_TIMEOUT_MS,
      roundTimeoutMs: ROUND_TIMEOUT_MS,
      hooks: {
        onApplied: this.bridge.onApplied,
        onDishonestDealer: () => this.onDishonest(),
        onActionUndelivered: () => this.flashNotice("Your action didn't reach the table - check your connection."),
        onResync: (reason) => this.onResync(reason),
      },
    });
    useGame.getState().connectLocal(this.bridge.build);
    this.watchStore();
    this.timer = setInterval(() => {
      if (!this.ended) this.node.tick(Date.now());
    }, TICK_MS);
  }

  /** Starts a session as the host: bootstrap peer, relay hub, and first seat. */
  static async host(cfg: NearbyConfig, name: string): Promise<NearbySession> {
    await initLocalCore();
    const selfId = encodePeerId(name);
    const s = new NearbySession(selfId, cfg, selfId);
    s.sit(0);
    return s;
  }

  /**
   * Joins an existing session from a host's offer blob. The host's config, if
   * present in the blob, overrides the caller's so this peer's core matches the
   * host's stakes/stack exactly; a blob without config is rejected so we never
   * silently build a divergent core.
   */
  static async join(
    name: string,
    offerBlob: string,
  ): Promise<{ session: NearbySession; answerBlob: string }> {
    await initLocalCore();
    const selfId = encodePeerId(name);
    const answerSession = await acceptOffer(selfId, offerBlob, NearbySession.rtcOptsFor(false));
    const hostCfg = readHostConfig(answerSession.offerPayload);
    if (!hostCfg) {
      answerSession.close();
      throw new Error("This invite is missing the table settings. Ask your host for a fresh invite.");
    }
    useNearby.getState().setConfig(hostCfg);
    const hostId = answerSession.offerPeerId ?? decodePeerId(offerBlob);
    const answerBlob = await answerSession.answerBlob();
    const session = new NearbySession(selfId, hostCfg, hostId);
    void answerSession.connection().then((conn) => session.onGuestConnected(conn)).catch(() => {
      session.flashNotice("Could not connect to the host. Try scanning the invite again.");
    });
    return { session, answerBlob };
  }

  private static rtcOptsFor(isHost: boolean): RtcOptions {
    return isHost ? {} : { onState: (s) => useNearby.getState().setConnectionState(CONN_STATE[s]) };
  }

  private rtcOpts(): RtcOptions {
    // Only a guest (single upstream link) drives the global indicator; the host
    // is the hub and stays "connected" as long as it retains any peer.
    return this.isHost ? {} : { onState: (s) => useNearby.getState().setConnectionState(CONN_STATE[s]) };
  }

  /** True when this peer is the host (relay hub); drives the reconnect UI shape. */
  get isHostRole(): boolean {
    return this.isHost;
  }

  /** Produces one invite for a joining (or reconnecting) guest, carrying config. */
  async createInvite(): Promise<HostInvite> {
    const offer = createOffer(this.selfId, this.rtcOpts(), configPayload(this.cfg));
    const offerBlob = await offer.offerBlob();
    return {
      offerBlob,
      accept: async (answerBlob: string) => {
        const conn = await offer.acceptAnswer(answerBlob);
        this.attach(conn);
      },
    };
  }

  /**
   * Guest side of a reconnect: accepts a fresh invite from the host into THIS
   * live session and re-attaches. The mesh heals the log via gossip catch-up, so
   * no explicit snapshot plumbing is needed. Returns the answer blob for the host.
   */
  async acceptReconnectOffer(offerBlob: string): Promise<string> {
    const answerSession = await acceptOffer(this.selfId, offerBlob, this.rtcOpts());
    const answerBlob = await answerSession.answerBlob();
    void answerSession.connection().then((conn) => this.attach(conn)).catch(() => {
      this.flashNotice("Reconnect failed. Try scanning the host's code again.");
    });
    return answerBlob;
  }

  private async onGuestConnected(conn: Connection): Promise<void> {
    this.attach(conn);
    // Catch up to the live log via gossip, then claim a free seat with retry.
    for (let i = 0; i < 50 && this.node.headSeq() === 0; i += 1) await sleep(100);
    await this.claimSeat();
  }

  /**
   * Claims the lowest free seat, retrying the next one if the core rejects the
   * sit (a stale seat map raced another joiner), bounded by the seat count, and
   * surfacing a clear message if the table is full (issue #28 hardening).
   */
  private async claimSeat(): Promise<void> {
    for (let attempt = 0; attempt < MAX_SEATS && !this.ended; attempt += 1) {
      const used = new Set(useGame.getState().seats.map((s) => s.seat));
      let seat = 0;
      while (seat < MAX_SEATS && used.has(seat)) seat += 1;
      if (seat >= MAX_SEATS) {
        this.flashNotice("This table is full - no free seat to take.");
        return;
      }
      this.sit(seat);
      if (await this.waitSeated(seat)) return;
    }
    if (!this.ended) this.flashNotice("Could not take a seat. Please try rejoining.");
  }

  /** Polls until we occupy `seat` or the confirmation window elapses. */
  private async waitSeated(seat: number): Promise<boolean> {
    const deadline = Date.now() + SEAT_CONFIRM_MS;
    while (Date.now() < deadline) {
      const mine = useGame.getState().seats.find((s) => s.playerId === this.selfId);
      if (mine) return mine.seat === seat;
      await sleep(150);
    }
    return false;
  }

  private attach(conn: Connection): void {
    this.closedPeers.delete(conn.peerId);
    this.node.attach(conn);
    this.conns.push(conn);
    conn.onClose(() => this.onPeerGone(conn.peerId));
    if (this.endTimer) {
      clearTimeout(this.endTimer);
      this.endTimer = null;
    }
    useNearby.getState().setConnectionState("connected");
  }

  private sit(seat: number): void {
    this.buyIns.set(this.selfId, this.cfg.startingStack);
    this.node.submitLocalAction({
      v: 1,
      type: "sit_down",
      data: { tableId: "nearby", seat, buyIn: this.cfg.startingStack },
    });
  }

  // ---- presence / departure ----

  private participantCount(): number {
    // Self plus every peer still within the mesh's alive window.
    return 1 + this.conns.filter((c) => !this.closedPeers.has(c.peerId)).length;
  }

  private onPeerGone(peerId: string): void {
    this.closedPeers.add(peerId);
    const name = displayName(peerId);
    const store = useNearby.getState();
    store.setReconnecting([...store.reconnecting.filter((n) => n !== name), name]);
    setTimeout(() => {
      const cur = useNearby.getState();
      cur.setReconnecting(cur.reconnecting.filter((n) => n !== name));
    }, RECONNECT_TOAST_MS);
    // Hold the seat for a grace window and offer a reconnect rather than ending
    // instantly the moment the last link drops.
    if (this.participantCount() < 2) {
      useNearby.getState().setConnectionState("lost");
      this.scheduleEndIfAlone();
    }
  }

  private scheduleEndIfAlone(): void {
    if (this.endTimer) return;
    this.endTimer = setTimeout(() => {
      this.endTimer = null;
      if (!this.ended && this.participantCount() < 2) this.end();
    }, RECONNECT_GRACE_MS);
  }

  private onVoid(): void {
    const { seats, buttonSeat } = useGame.getState();
    const dealer = seats.find((s) => s.seat === buttonSeat);
    const who = dealer?.name ?? "the dealer";
    useNearby.getState().setVoidToast(`Hand voided - ${who} was dealing`);
  }

  private onDishonest(): void {
    const handId = useGame.getState().handId;
    if (handId) useNearby.getState().setDishonest(handId);
  }

  private onResync(reason: "divergence" | "unauthorized" | "recovered"): void {
    if (reason === "recovered") {
      this.flashNotice("Resynced with the table.");
      return;
    }
    useNearby.getState().setNotice("Resyncing with the table...");
  }

  private flashNotice(msg: string): void {
    useNearby.getState().setNotice(msg);
    setTimeout(() => {
      const cur = useNearby.getState();
      if (cur.notice === msg) cur.setNotice(null);
    }, NOTICE_MS);
  }

  // ---- summary bookkeeping ----

  private watchStore(): void {
    let prevHandId = useGame.getState().handId;
    this.unsub = useGame.subscribe((s) => {
      if (s.handId && s.handId !== prevHandId) {
        prevHandId = s.handId;
        this.handsPlayed += 1;
      }
      if (s.pot > this.biggestPot) this.biggestPot = s.pot;
      for (const seat of s.seats) {
        if (!this.buyIns.has(seat.playerId)) this.buyIns.set(seat.playerId, this.cfg.startingStack);
      }
      // Subtle "shuffling together" micro-state: between hands with a table.
      const active = s.seats.filter((x) => !x.sittingOut && x.stack > 0).length;
      useNearby.getState().setShuffling(!s.handRunning && active >= 2 && s.street !== "showdown");
    });
  }

  private summaryRows(): SummaryRow[] {
    return useGame
      .getState()
      .seats.map((seat) => {
        const buyIn = this.buyIns.get(seat.playerId) ?? this.cfg.startingStack;
        return {
          playerId: seat.playerId,
          name: seat.name || displayName(seat.playerId),
          buyIn,
          finalStack: seat.stack,
          net: seat.stack - buyIn,
        };
      })
      .sort((a, b) => b.net - a.net);
  }

  /** Ends the session and shows the summary. Idempotent. */
  end(): void {
    if (this.ended) return;
    this.ended = true;
    const nearby = useNearby.getState();
    nearby.setSummary({
      handsPlayed: this.handsPlayed,
      biggestPot: this.biggestPot,
      rows: this.summaryRows(),
    });
    nearby.setShuffling(false);
    nearby.setPhase("summary");
    this.teardown();
  }

  /** Full teardown without a summary (e.g. leaving before any hand). */
  dispose(): void {
    this.ended = true;
    this.teardown();
  }

  private teardown(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
    if (this.endTimer) clearTimeout(this.endTimer);
    this.endTimer = null;
    this.unsub?.();
    this.unsub = null;
    for (const c of this.conns) c.close();
    useGame.getState().disconnect();
  }
}

/** The host config embedded in an invite blob so guests build an identical core. */
function configPayload(cfg: NearbyConfig): Record<string, unknown> {
  return { nearby: cfg };
}

/** Recovers a host NearbyConfig from a decoded invite payload, or null. */
function readHostConfig(payload: Record<string, unknown> | undefined): NearbyConfig | null {
  const raw = payload?.nearby as Partial<NearbyConfig> | undefined;
  if (!raw || typeof raw.smallBlind !== "number" || typeof raw.bigBlind !== "number") return null;
  return {
    tableName: typeof raw.tableName === "string" ? raw.tableName : "Nearby table",
    smallBlind: raw.smallBlind,
    bigBlind: raw.bigBlind,
    startingStack: typeof raw.startingStack === "number" ? raw.startingStack : 200,
  };
}

/** Reads the peer id embedded in an rtc.ts base64 signal blob. */
function decodePeerId(blob: string): string {
  try {
    return (JSON.parse(atob(blob)) as { peerId: string }).peerId;
  } catch {
    return "host";
  }
}
