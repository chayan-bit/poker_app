// Construction-time types and tuning constants for MeshNode (issue #28), split
// out of mesh.ts to keep that file focused on the state machine itself.

import type { EventMap, LocalConfig } from "./core.ts";
import type { MeshView } from "./meshview.ts";
import type { Connection } from "./transport.ts";
import type { LogEntry } from "./wire.ts";

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
}

export interface MeshOptions {
  selfId: string;
  core: CoreLike;
  config: LocalConfig;
  connections: Connection[];
  /** Control-plane clock (ms). Never fed to the core except via stamped entries. */
  clock: () => number;
  /** Peer id that bootstraps sequencing before any seat is occupied. */
  bootstrapId: string;
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
/** Event types that mean real hand progress, so they reset the turn timer. */
export const PROGRESS_EVENTS = ["hand_dealt", "bet_placed", "street_advanced", "showdown"];
