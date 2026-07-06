// Wire shapes for the replicated-state-machine mesh (issue #28). Everything a
// peer sends is one of these, JSON-serialized. The ordered action log is the
// heart of the protocol: replicating the log replicates the whole game, because
// the #27 core is a pure function of (state, ordered actions, seed).

import type { Envelope, LocalConfig } from "./core.ts";

/** How a single log entry mutates the local core when applied. */
export type EntryKind = "submit" | "seed" | "tick" | "void";

/**
 * One totally-ordered input to every peer's core. `seq` is assigned by the
 * hand's coordinator and is contiguous from 1. `hashAfter` is the coordinator's
 * core state hash immediately after applying this entry; every peer recomputes
 * its own hash and rejects the entry (flags divergence) on mismatch, so the
 * coordinator holds no special trust beyond assigning order.
 */
export interface LogEntry {
  seq: number;
  kind: EntryKind;
  /** Player id for a "submit"; "" for coordinator/system entries. */
  actor: string;
  /** Present for kind "submit": the exact command handed to core.submit. */
  envelope?: Envelope;
  /** Present for kind "seed": the 64-hex multi-party seed for the next hand. */
  seedHex?: string;
  /** Coordinator timestamp for kind "tick" and for fold_on_timeout submits. */
  nowMs?: number;
  /** Coordinator's core state hash after applying this entry. */
  hashAfter: string;
  /** Peer id of the coordinator that sequenced this entry. */
  by: string;
}

/** A peer's proposed action, sent to the current coordinator for sequencing. */
export interface ActionRequest {
  actor: string;
  envelope: Envelope;
}

/**
 * A full-history snapshot served to a late joiner. The joiner replays every
 * entry into a fresh core and checks the resulting hash equals `stateHash`,
 * then cross-checks `stateHash` against a second peer before trusting it.
 * Full-log replay (rather than opaque state) keeps the sync verifiable; log
 * compaction is future work (noted in the report).
 */
export interface Snapshot {
  config: LocalConfig;
  entries: LogEntry[];
  head: number;
  stateHash: string;
}

/** Fair-seed sub-protocol phases (see fairmp.ts). */
export type FairPhase =
  | "commit_req"
  | "commit"
  | "reveal_req"
  | "reveal"
  | "seed";

/** One message of the multi-party seed round, relayed like any mesh frame. */
export interface FairMsg {
  t: "fair";
  from: string;
  phase: FairPhase;
  hand: number;
  /** participant player ids, ordered, for commit_req / seed. */
  participants?: string[];
  /** SHA-256(share) hex for phase "commit". */
  commit?: string;
  /** 64-hex share for phase "reveal". */
  shareHex?: string;
  /** Revealed shares keyed by player id for phase "seed" (post-round verify). */
  shares?: Record<string, string>;
  /** The combined 64-hex seed the coordinator will log, for phase "seed". */
  seedHex?: string;
}

/** All frames exchanged on the mesh. */
export type MeshMsg =
  | { t: "hello"; from: string }
  | { t: "heartbeat"; from: string; head: number; nowMs: number; coordSeat: number }
  | { t: "entries"; from: string; entries: LogEntry[] }
  | { t: "need"; from: string; have: number }
  | { t: "request"; from: string; req: ActionRequest }
  | { t: "snapshot_req"; from: string }
  | { t: "snapshot"; from: string; snap: Snapshot }
  | FairMsg;
