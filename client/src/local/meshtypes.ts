// Construction-time types and tuning constants for MeshNode (issue #28), split
// out of mesh.ts to keep that file focused on the state machine itself.

import type { EventMap, LocalConfig } from "./core.ts";
import type { MeshView } from "./meshview.ts";
import type { Connection } from "./transport.ts";
import type { ActionRequest, LogEntry } from "./wire.ts";

/** The subset of the #27 core the mesh drives. LocalCore satisfies it. */
export interface CoreLike {
  submit(playerId: string, envelope: string): EventMap;
  tick(nowMs: number): EventMap;
  stateHash(): string;
  voidHand(): EventMap;
  setSeed(seedHex: string): void;
}

export interface MeshHooks {
  onDivergence?(entry: LogEntry, localHash: string): void;
  onDishonestDealer?(seedHex: string): void;
  onApplied?(entry: LogEntry, events: EventMap, view: MeshView): void;
  /** Fired when the node rejects a divergent/unauthorized entry and requests a
   *  fresh snapshot to recover, and again once it has adopted one. */
  onResync?(reason: "divergence" | "unauthorized" | "recovered"): void;
  /** Fired when a local action could not be delivered to the coordinator after
   *  the bounded retries, so the UI can prompt the player to try again. */
  onActionUndelivered?(req: ActionRequest): void;
}

export interface MeshOptions {
  selfId: string;
  core: CoreLike;
  config: LocalConfig;
  connections: Connection[];
  /** Control-plane clock (ms). Never fed to the core except via stamped entries. */
  clock: () => number;
  /** Peer id that bootstraps sequencing before any seat is occupied. Also the
   *  relay hub: the peer whose selfId equals bootstrapId fans out broadcasts and
   *  forwards directed frames between peers that cannot reach each other (star). */
  bootstrapId: string;
  /**
   * Builds a fresh core to recover from a rejected divergent entry: the WASM
   * core cannot roll back a bad mutation, so resync rebuilds from a peer
   * snapshot into a clean core. Omit in tests that never diverge; adoptSnapshot
   * then replays into the (fresh) core it was given.
   */
  makeCore?: () => CoreLike;
  heartbeatMs?: number;
  graceMs?: number;
  turnTimeoutMs?: number;
  roundTimeoutMs?: number;
  hooks?: MeshHooks;
}

export const DEFAULT_HEARTBEAT = 500;
export const DEFAULT_GRACE = 2000;
export const DEFAULT_TURN_TIMEOUT = 20_000;
export const DEFAULT_ROUND_TIMEOUT = 3000;
/** Resend an unacked action request after this long, up to MAX_ACTION_TRIES. */
export const ACTION_RETRY_MS = 1200;
export const MAX_ACTION_TRIES = 5;
/** Event types that mean real hand progress, so they reset the turn timer. */
export const PROGRESS_EVENTS = ["hand_dealt", "bet_placed", "street_advanced", "showdown"];
