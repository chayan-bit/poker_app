// Per-hand fair-seed round driver (issue #28), split out of mesh.ts.
//
// This owns the commit/reveal state machine that turns each hand's multi-party
// shares into the seed the coordinator logs. It is a tick-driven machine so it
// cannot wedge on a single participant or a lost transition message. The mesh
// delegates to it: the coordinator calls stepCoordinator each beat, and every
// peer routes "fair" frames through handle so it can play the participant role
// (respond to commit_req / reveal_req) regardless of who coordinates.

import { combineSeedHex, commit, randomShare, SeedRound, shareToHex } from "./fairmp.ts";
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

export class FairSeedDriver {
  private round: SeedRound | null = null;
  private roundStartedMs = 0;
  private roundId = 0;
  private revealRequested = false;
  private readonly myShares = new Map<number, Uint8Array>();
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
    this.myShares.set(round.hand, share);
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
      participants: round.order(), shares: round.revealedSharesHex(), seedHex,
    });
    this.ctx.onSeed(seedHex);
  }

  /** Routes one fair frame. `isCoordinator` gates the collector transitions. */
  async handle(msg: FairMsg, isCoordinator: boolean): Promise<void> {
    switch (msg.phase) {
      case "commit_req": {
        if (!msg.participants?.includes(this.ctx.selfId)) return;
        const share = randomShare();
        this.myShares.set(msg.hand, share);
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
        const share = this.myShares.get(msg.hand);
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
        const expected = await combineSeedHex(msg.participants, msg.shares);
        if (expected !== msg.seedHex) this.ctx.onDishonest(msg.seedHex);
        else this.expectedSeeds.add(msg.seedHex);
        break;
      }
    }
  }
}
