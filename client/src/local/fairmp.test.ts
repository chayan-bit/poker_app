// Unit tests for the multi-party fair-seed protocol. Runs in plain Node:
// WebCrypto (crypto.subtle / getRandomValues) is global in Node 20+.
//
// Fixed SHA-256 vectors (independently computed):
//   sha256(0x00 * 32)            = 66687aad...
//   sha256(0x01 * 32)            = 72cd6e84...
//   sha256(0x00*32 || 0x01*32)   = 5c85955f...
//   sha256(0x01*32 || 0x00*32)   = 037d6dfb...
//   sha256(0x01 0x02 0x03)       = 039058c6...

import { describe, expect, it } from "vitest";
import {
  combineSeed,
  combineSeedHex,
  commit,
  genHandKeyPair,
  openHoleCards,
  randomShare,
  sealHoleCards,
  SeedRound,
  shareFromHex,
  shareToHex,
  verifyCommit,
} from "./fairmp";

const ZEROS = new Uint8Array(32);
const ONES = new Uint8Array(32).fill(1);

const COMMIT_ZEROS = "66687aadf862bd776c8fc18b8e9f8e20089714856ee233b3902a591d0d5f2925";
const COMMIT_ONES = "72cd6e8422c407fb6d098690f1130b7ded7ec2f7f5e1d30bd9d521f015363793";
const SEED_ZEROS_ONES = "5c85955f709283ecce2b74f1b1552918819f390911816e7bb466805a38ab87f3";
const SEED_ONES_ZEROS = "037d6dfb3a369a41e01100fdd53c35ee3fb69ddec5830d61e1138d066a4c2285";
const SEED_010203 = "039058c6f2c0cb492c533b0a4d14ef77cc0f78abccced5287d84a1a2011cfb81";

describe("randomShare", () => {
  it("draws a 32-byte share", () => {
    expect(randomShare()).toHaveLength(32);
  });

  it("draws distinct shares on consecutive calls", () => {
    expect(shareToHex(randomShare())).not.toBe(shareToHex(randomShare()));
  });
});

describe("share hex codec", () => {
  it("round-trips a share through hex", () => {
    const share = randomShare();
    expect(shareFromHex(shareToHex(share))).toEqual(share);
  });

  it("encodes bytes as two lowercase hex chars each", () => {
    expect(shareToHex(new Uint8Array([0, 1, 255]))).toBe("0001ff");
  });

  it("throws on odd-length hex input", () => {
    expect(() => shareFromHex("abc")).toThrow(/odd-length hex/);
  });
});

describe("commit / verifyCommit", () => {
  it("commits to SHA-256 of the share (fixed vector)", async () => {
    expect(await commit(ZEROS)).toBe(COMMIT_ZEROS);
    expect(await commit(ONES)).toBe(COMMIT_ONES);
  });

  it("accepts a share that opens its commitment", async () => {
    expect(await verifyCommit(ZEROS, COMMIT_ZEROS)).toBe(true);
  });

  it("rejects a share that does not open the commitment", async () => {
    expect(await verifyCommit(ONES, COMMIT_ZEROS)).toBe(false);
  });
});

describe("combineSeed", () => {
  it("combines shares as SHA-256 of their concatenation (fixed vector)", async () => {
    expect(await combineSeed([ZEROS, ONES])).toBe(SEED_ZEROS_ONES);
  });

  it("is order-sensitive: swapping participants changes the seed", async () => {
    expect(await combineSeed([ONES, ZEROS])).toBe(SEED_ONES_ZEROS);
  });

  it("combines hex shares in the given player order", async () => {
    const seed = await combineSeedHex(["alice", "bob", "carol"], {
      alice: "01",
      bob: "02",
      carol: "03",
    });
    expect(seed).toBe(SEED_010203);
  });

  it("returns 64 hex chars ready for core.setSeed", async () => {
    expect(await combineSeed([randomShare()])).toMatch(/^[0-9a-f]{64}$/);
  });
});

describe("SeedRound", () => {
  it("fixes the participant order by sorting player ids", () => {
    const round = new SeedRound(1, ["bob", "alice", "carol"]);
    expect(round.order()).toEqual(["alice", "bob", "carol"]);
  });

  it("ignores commits from players outside the participant list", async () => {
    const round = new SeedRound(1, ["alice", "bob"]);
    round.addCommit("mallory", await commit(ZEROS));
    round.addCommit("alice", await commit(ZEROS));
    expect(round.allCommitted()).toBe(false);
  });

  it("reports all committed only once every participant has committed", async () => {
    const round = new SeedRound(1, ["alice", "bob"]);
    round.addCommit("alice", await commit(ZEROS));
    expect(round.allCommitted()).toBe(false);
    round.addCommit("bob", await commit(ONES));
    expect(round.allCommitted()).toBe(true);
  });

  it("returns unknown for a reveal without a prior commit", async () => {
    const round = new SeedRound(1, ["alice", "bob"]);
    expect(await round.addReveal("alice", shareToHex(ZEROS))).toBe("unknown");
  });

  it("returns bad for a reveal that does not open its commit", async () => {
    const round = new SeedRound(1, ["alice"]);
    round.addCommit("alice", await commit(ZEROS));
    expect(await round.addReveal("alice", shareToHex(ONES))).toBe("bad");
    expect(round.allRevealed()).toBe(false);
  });

  it("derives the seed from the revealed shares in participant order", async () => {
    // Sorted order is [alice, bob]; alice holds ZEROS, bob holds ONES.
    const round = new SeedRound(7, ["bob", "alice"]);
    round.addCommit("alice", await commit(ZEROS));
    round.addCommit("bob", await commit(ONES));
    expect(await round.addReveal("bob", shareToHex(ONES))).toBe("ok");
    expect(await round.addReveal("alice", shareToHex(ZEROS))).toBe("ok");
    expect(round.allRevealed()).toBe(true);
    expect(await round.seed()).toBe(SEED_ZEROS_ONES);
    expect(round.revealedSharesHex()).toEqual({
      alice: shareToHex(ZEROS),
      bob: shareToHex(ONES),
    });
  });

  it("keeps everyone when all participants revealed", async () => {
    const round = new SeedRound(1, ["alice"]);
    round.addCommit("alice", await commit(ZEROS));
    await round.addReveal("alice", shareToHex(ZEROS));
    expect(round.excludeMissing()).toBe(false);
    expect(round.order()).toEqual(["alice"]);
  });

  it("excludes a non-revealer and restarts the round with the remaining committers", async () => {
    const round = new SeedRound(3, ["alice", "bob", "carol"]);
    round.addCommit("alice", await commit(ZEROS));
    round.addCommit("bob", await commit(ONES));
    round.addCommit("carol", "deadbeef");
    await round.addReveal("alice", shareToHex(ZEROS));
    await round.addReveal("bob", shareToHex(ONES));
    // carol never reveals.
    expect(round.allRevealed()).toBe(false);

    expect(round.excludeMissing()).toBe(true);
    expect(round.order()).toEqual(["alice", "bob"]);
    // The round restarts: commits and shares are wiped for a fresh commit round.
    expect(round.allCommitted()).toBe(false);

    round.addCommit("alice", await commit(ZEROS));
    round.addCommit("bob", await commit(ONES));
    await round.addReveal("alice", shareToHex(ZEROS));
    await round.addReveal("bob", shareToHex(ONES));
    expect(round.allRevealed()).toBe(true);
    expect(await round.seed()).toBe(SEED_ZEROS_ONES);
  });
});

describe("per-hand hole-card envelopes", () => {
  it("round-trips a hole-card envelope between dealer and recipient", async () => {
    const dealer = await genHandKeyPair();
    const player = await genHandKeyPair();
    const sealed = await sealHoleCards(dealer.privateKey, player.publicJwk, '["As","Kd"]');
    const opened = await openHoleCards(player.privateKey, dealer.publicJwk, sealed);
    expect(opened).toBe('["As","Kd"]');
  });

  it("produces a fresh IV per envelope so identical plaintexts differ on the wire", async () => {
    const dealer = await genHandKeyPair();
    const player = await genHandKeyPair();
    const a = await sealHoleCards(dealer.privateKey, player.publicJwk, "x");
    const b = await sealHoleCards(dealer.privateKey, player.publicJwk, "x");
    expect(a.iv).not.toBe(b.iv);
  });

  it("rejects opening with the wrong recipient key", async () => {
    const dealer = await genHandKeyPair();
    const player = await genHandKeyPair();
    const eavesdropper = await genHandKeyPair();
    const sealed = await sealHoleCards(dealer.privateKey, player.publicJwk, "secret");
    await expect(
      openHoleCards(eavesdropper.privateKey, dealer.publicJwk, sealed),
    ).rejects.toThrow();
  });
});
