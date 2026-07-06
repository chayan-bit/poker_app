// The nearby (offline P2P) session controller. It assembles the real #27 table
// core, the #28 replicated mesh, and the store bridge into one live session, and
// drives the serverless WebRTC signaling. It is the single owner of the mesh
// node lifecycle and the only writer of nearby UI state.
//
// Assembly mirrors simulation.mts, but with real WebRTC links instead of the
// in-memory net and a wall clock instead of a virtual one. The host is the
// bootstrap peer and first seat; guests join by exchanging offer/answer blobs
// and catch up through the mesh's own need/entries gossip (no snapshot plumbing
// required), then take a free seat.

import { LocalCore, initLocalCore, type LocalConfig } from "@/local/core";
import { MeshNode } from "@/local/mesh";
import { MeshBridge } from "@/local/storeBridge";
import { acceptOffer, createOffer } from "@/local/rtc";
import type { Connection } from "@/local/transport";
import { useGame } from "@/store/gameStore";
import { displayName, encodePeerId } from "./names";
import { useNearby, type NearbyConfig, type SummaryRow } from "./nearbyStore";

const TICK_MS = 250;
const TURN_TIMEOUT_MS = 30_000;
const ROUND_TIMEOUT_MS = 4_000;
const GRACE_MS = 6_000;
const RECONNECT_TOAST_MS = 5_000;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function meshConfig(c: NearbyConfig): LocalConfig {
  return {
    id: "nearby",
    maxSeats: 9,
    smallBlind: c.smallBlind,
    bigBlind: c.bigBlind,
    autoStart: true,
    turnTimeoutMs: TURN_TIMEOUT_MS,
    disconnectGraceMs: GRACE_MS,
  };
}

/** An in-progress host invite: the offer blob to share and a way to accept the
 *  scanned/pasted answer that comes back. */
export interface HostInvite {
  offerBlob: string;
  accept: (answerBlob: string) => Promise<void>;
}

export class NearbySession {
  readonly selfId: string;
  private readonly cfg: NearbyConfig;
  private readonly node: MeshNode;
  private readonly bridge: MeshBridge;
  private readonly conns: Connection[] = [];
  private readonly buyIns = new Map<string, number>();
  private timer: ReturnType<typeof setInterval> | null = null;
  private unsub: (() => void) | null = null;
  private handsPlayed = 0;
  private biggestPot = 0;
  private ended = false;

  private constructor(selfId: string, cfg: NearbyConfig, bootstrapId: string) {
    this.selfId = selfId;
    this.cfg = cfg;
    const config = meshConfig(cfg);
    const core = new LocalCore(config, "");
    this.bridge = new MeshBridge({
      selfId,
      config,
      nameFor: displayName,
      submit: (env) => this.node.submitLocalAction(env),
      onVoid: () => this.onVoid(),
    });
    this.node = new MeshNode({
      selfId,
      core,
      config,
      connections: [],
      clock: () => Date.now(),
      bootstrapId,
      graceMs: GRACE_MS,
      turnTimeoutMs: TURN_TIMEOUT_MS,
      roundTimeoutMs: ROUND_TIMEOUT_MS,
      hooks: {
        onApplied: this.bridge.onApplied,
        onDishonestDealer: () => this.onDishonest(),
      },
    });
    useGame.getState().connectLocal(this.bridge.build);
    this.watchStore();
    this.timer = setInterval(() => {
      if (!this.ended) this.node.tick(Date.now());
    }, TICK_MS);
  }

  /** Starts a session as the host: bootstrap peer and first seat. */
  static async host(cfg: NearbyConfig, name: string): Promise<NearbySession> {
    await initLocalCore();
    const selfId = encodePeerId(name);
    const s = new NearbySession(selfId, cfg, selfId);
    s.sit(0);
    return s;
  }

  /** Joins an existing session from a host's offer blob, returning the answer
   *  blob to hand back to the host and the live session. */
  static async join(
    cfg: NearbyConfig,
    name: string,
    offerBlob: string,
  ): Promise<{ session: NearbySession; answerBlob: string }> {
    await initLocalCore();
    const hostId = decodePeerId(offerBlob);
    const selfId = encodePeerId(name);
    const answerSession = await acceptOffer(selfId, offerBlob);
    const answerBlob = await answerSession.answerBlob();
    const session = new NearbySession(selfId, cfg, hostId);
    void answerSession.connection().then((conn) => session.onGuestConnected(conn));
    return { session, answerBlob };
  }

  /** Produces one invite for a joining guest. Call again per additional guest. */
  async createInvite(): Promise<HostInvite> {
    const offer = createOffer(this.selfId);
    const offerBlob = await offer.offerBlob();
    return {
      offerBlob,
      accept: async (answerBlob: string) => {
        const conn = await offer.acceptAnswer(answerBlob);
        this.attach(conn);
      },
    };
  }

  private async onGuestConnected(conn: Connection): Promise<void> {
    this.attach(conn);
    // Catch up to the live log via gossip, then claim the lowest free seat.
    for (let i = 0; i < 50 && this.node.headSeq() === 0; i++) await sleep(100);
    const used = new Set(useGame.getState().seats.map((s) => s.seat));
    let seat = 0;
    while (used.has(seat)) seat += 1;
    this.sit(seat);
  }

  private attach(conn: Connection): void {
    this.node.attach(conn);
    this.conns.push(conn);
    conn.onClose(() => this.onPeerGone(conn.peerId));
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
    return 1 + this.conns.filter((c) => this.isOpen(c)).length;
  }

  private isOpen(conn: Connection): boolean {
    return !this.closedPeers.has(conn.peerId);
  }

  private readonly closedPeers = new Set<string>();

  private onPeerGone(peerId: string): void {
    this.closedPeers.add(peerId);
    const name = displayName(peerId);
    const store = useNearby.getState();
    store.setReconnecting([...store.reconnecting.filter((n) => n !== name), name]);
    setTimeout(() => {
      const cur = useNearby.getState();
      cur.setReconnecting(cur.reconnecting.filter((n) => n !== name));
    }, RECONNECT_TOAST_MS);
    // The session ends only when fewer than 2 participants remain.
    if (this.participantCount() < 2) this.end();
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
    this.unsub?.();
    this.unsub = null;
    for (const c of this.conns) c.close();
    useGame.getState().disconnect();
  }
}

/** Reads the peer id embedded in an rtc.ts base64 signal blob. */
function decodePeerId(blob: string): string {
  try {
    return (JSON.parse(atob(blob)) as { peerId: string }).peerId;
  } catch {
    return "host";
  }
}
