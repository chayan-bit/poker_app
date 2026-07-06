// Per-hand fair-seed round driver (issue #28), split out of mesh.ts.
//
// This owns the commit/reveal state machine that turns each hand's multi-party
// shares into the seed the coordinator logs. It is a tick-driven machine so it
// cannot wedge on a single participant or a lost transition message. The mesh
// delegates to it: the coordinator calls stepCoordinator each beat, and every
// peer routes "fair" frames through handle so it can play the participant role
// (respond to commit_req / reveal_req) regardless of who coordinates.

import {
  combineSeedHex,
  commit,
  randomShare,
  SeedRound,
  shareFromHex,
  shareToHex,
  verifyCommit,
} from "./fairmp.ts";
import type { FairMsg, MeshMsg } from "./wire.ts";

export interface FairCtx {
  selfId: string;
  roundTimeoutMs: number;
  broadcast(msg: MeshMsg): void;
  sendTo(peer: string, msg: MeshMsg): void;
  onDishonest(seedHex: string): void;
  /** The finalized seed; the mesh logs it (setSeed) and nudges the deal. */
  onSeed(seedHex: string): void;
}

/**
 * Round-scoped key for a peer's own share. The round counter (`hand`) is local
 * to each coordinator, so it collides across coordinators and across a peer's
 * own coordinator/participant roles; namespacing by the coordinator id makes the
 * key globally unique, which the own-share dishonesty check depends on.
 */
function roundKey(coordinatorId: string, hand: number): string {
  return `${coordinatorId}:${hand}`;
}

export class FairSeedDriver {
  private round: SeedRound | null = null;
  private roundStartedMs = 0;
  private roundId = 0;
  private revealRequested = false;
  private readonly myShares = new Map<string, Uint8Array>();
  private readonly expectedSeeds = new Set<string>();

  constructor(private readonly ctx: FairCtx) {}

  /** True once a round is in flight, so the mesh stops re-triggering it. */
  get active(): boolean {
    return this.round !== null;
  }

  /** Abandons any in-flight round (e.g. when this peer stops coordinating). */
  abandon(): void {
    this.round = null;
    this.revealRequested = false;
  }

  /** One coordinator beat: create, advance, finalize, or time out the round. */
  stepCoordinator(now: number, participants: string[]): void {
    if (!this.round) {
      this.roundId += 1;
      this.round = new SeedRound(this.roundId, participants);
      this.roundStartedMs = now;
      this.revealRequested = false;
      void this.startCommit();
      return;
    }
    const round = this.round;
    if (round.allRevealed()) {
      void this.finishRound(round);
      return;
    }
    if (round.allCommitted() && !this.revealRequested) {
      this.revealRequested = true;
      this.ctx.broadcast({ t: "fair", from: this.ctx.selfId, phase: "reveal_req", hand: round.hand });
      return;
    }
    if (now - this.roundStartedMs > this.ctx.roundTimeoutMs && !round.allRevealed()) {
      if (round.excludeMissing()) {
        this.roundStartedMs = now;
        this.revealRequested = false;
        void this.startCommit();
      }
    }
  }

  private async startCommit(): Promise<void> {
    const round = this.round;
    if (!round) return;
    const share = randomShare();
    const shareHex = shareToHex(share);
    this.myShares.set(roundKey(this.ctx.selfId, round.hand), share);
    round.addCommit(this.ctx.selfId, await commit(share));
    // The coordinator opens its own commit immediately so a single-participant
    // round can complete; only peers' reveals are awaited over the wire.
    await round.addReveal(this.ctx.selfId, shareHex);
    this.ctx.broadcast({
      t: "fair", from: this.ctx.selfId, phase: "commit_req", hand: round.hand, participants: round.order(),
    });
  }

  private async finishRound(round: SeedRound): Promise<void> {
    // Claim the round synchronously so a second concurrent reveal cannot
    // double-deal (which would strand the mesh's dealing flag).
    if (this.round !== round) return;
    this.round = null;
    const seedHex = await round.seed();
    this.expectedSeeds.add(seedHex);
    this.ctx.broadcast({
      t: "fair", from: this.ctx.selfId, phase: "seed", hand: round.hand,
      participants: round.order(), shares: round.revealedSharesHex(),
      commits: round.commitsHex(), seedHex,
    });
    this.ctx.onSeed(seedHex);
  }

  /** Routes one fair frame. `isCoordinator` gates the collector transitions. */
  async handle(msg: FairMsg, isCoordinator: boolean): Promise<void> {
    switch (msg.phase) {
      case "commit_req": {
        if (!msg.participants?.includes(this.ctx.selfId)) return;
        // Idempotent under relay/duplicate delivery: reuse this hand's share so a
        // second commit_req cannot make us commit to a different value than we
        // will later reveal (which would look dishonest to the coordinator).
        const key = roundKey(msg.from, msg.hand);
        let share = this.myShares.get(key);
        if (!share) {
          share = randomShare();
          this.myShares.set(key, share);
        }
        this.ctx.sendTo(msg.from, {
          t: "fair", from: this.ctx.selfId, phase: "commit", hand: msg.hand, commit: await commit(share),
        });
        break;
      }
      case "commit": {
        const round = this.round;
        if (!round || !isCoordinator || msg.hand !== round.hand) return;
        round.addCommit(msg.from, msg.commit ?? "");
        break;
      }
      case "reveal_req": {
        const share = this.myShares.get(roundKey(msg.from, msg.hand));
        if (!share) return;
        this.ctx.sendTo(msg.from, {
          t: "fair", from: this.ctx.selfId, phase: "reveal", hand: msg.hand, shareHex: shareToHex(share),
        });
        break;
      }
      case "reveal": {
        const round = this.round;
        if (!round || !isCoordinator || msg.hand !== round.hand) return;
        await round.addReveal(msg.from, msg.shareHex ?? "");
        break;
      }
      case "seed": {
        if (!msg.seedHex || !msg.participants || !msg.shares) return;
        const mine = this.myShares.get(roundKey(msg.from, msg.hand));
        if (await this.seedIsHonest(mine, msg.participants, msg.shares, msg.commits, msg.seedHex)) {
          this.expectedSeeds.add(msg.seedHex);
        } else {
          this.ctx.onDishonest(msg.seedHex);
        }
        break;
      }
    }
  }

  /**
   * A finalized seed is honest for THIS participant only if:
   *  1. every revealed share opens its published commitment (no substitution),
   *  2. our own committed share is present and byte-identical in the reveal set
   *     (the dealer did not silently drop or replace it to bias the deck),
   *  3. the combination of the revealed shares reproduces the logged seed.
   * Any failure returns false so the caller flags the dishonest dealer.
   */
  private async seedIsHonest(
    mine: Uint8Array | undefined,
    participants: string[],
    shares: Record<string, string>,
    commits: Record<string, string> | undefined,
    seedHex: string,
  ): Promise<boolean> {
    for (const p of participants) {
      const shareHex = shares[p];
      if (!shareHex) return false;
      const commitHex = commits?.[p];
      if (commitHex && !(await verifyCommit(shareFromHex(shareHex), commitHex))) return false;
    }
    if (mine && participants.includes(this.ctx.selfId)) {
      if (shares[this.ctx.selfId] !== shareToHex(mine)) return false;
    }
    return (await combineSeedHex(participants, shares)) === seedHex;
  }
}
