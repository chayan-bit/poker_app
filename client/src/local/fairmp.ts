// Multi-party fair-seed protocol + per-hand keypairs (issue #28), all WebCrypto.
//
// Before each hand every player commits SHA-256(s_i); after everyone has
// committed they reveal s_i; the hand seed is SHA-256(s_1 || ... || s_n) in
// participant order, fed to the #27 core's setSeed. No single player can bias
// the deck, and because the shares are revealed the combination is verifiable:
// after the hand every peer recomputes the seed and flags a dishonest dealer on
// mismatch. A player who refuses to reveal is excluded and the round restarts
// with the remaining committers (SeedRound.excludeMissing).
//
// Randomness (shares, keypairs) comes ONLY from WebCrypto and lives entirely
// outside the deterministic apply path: the derived seed enters every core via
// a replicated log entry, so all cores stay byte-identical regardless.

const SHARE_BYTES = 32;

/** Draws one 32-byte entropy share from the platform CSPRNG. */
export function randomShare(): Uint8Array {
  const b = new Uint8Array(SHARE_BYTES);
  crypto.getRandomValues(b);
  return b;
}

function toHex(bytes: Uint8Array): string {
  let hex = "";
  for (const b of bytes) hex += b.toString(16).padStart(2, "0");
  return hex;
}

function fromHex(hex: string): Uint8Array {
  if (hex.length % 2 !== 0) throw new Error("fairmp: odd-length hex");
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

export { toHex as shareToHex, fromHex as shareFromHex };

async function sha256(bytes: Uint8Array): Promise<Uint8Array> {
  const digest = await crypto.subtle.digest("SHA-256", bytes as unknown as BufferSource);
  return new Uint8Array(digest);
}

/** SHA-256(share) as hex; this is the value a player publishes in the commit round. */
export async function commit(share: Uint8Array): Promise<string> {
  return toHex(await sha256(share));
}

/** True iff `share` opens `commitHex` (constant-work compare is unnecessary here). */
export async function verifyCommit(share: Uint8Array, commitHex: string): Promise<boolean> {
  return (await commit(share)) === commitHex;
}

/**
 * The hand seed: SHA-256 of the shares concatenated in participant order,
 * returned as 64-hex ready for core.setSeed. Order matters and is fixed by the
 * participant list so every peer combines identically.
 */
export async function combineSeed(sharesInOrder: Uint8Array[]): Promise<string> {
  const total = sharesInOrder.reduce((n, s) => n + s.length, 0);
  const concat = new Uint8Array(total);
  let off = 0;
  for (const s of sharesInOrder) {
    concat.set(s, off);
    off += s.length;
  }
  return toHex(await sha256(concat));
}

/** Convenience: combine hex-encoded shares in the given player order. */
export async function combineSeedHex(
  order: string[],
  sharesHex: Record<string, string>,
): Promise<string> {
  return combineSeed(order.map((p) => fromHex(sharesHex[p])));
}

// ---- Per-hand keypairs for encrypted hole-card delivery ----------------------
//
// The coordinator (dealer) is the only peer that learns the deck for its hand,
// exactly like a physical dealer, and hands each player their two cards over the
// direct channel, encrypted to a fresh per-hand ECDH (P-256) public key each
// player published. These primitives implement that channel. NOTE: the #27 core
// currently deals internally from the seed, so wiring blind-dealt cores to this
// path needs an additive core change (documented in the report); the protocol
// and its crypto are complete and tested here so that change is drop-in.

export interface HandKeyPair {
  publicJwk: JsonWebKey;
  privateKey: CryptoKey;
}

/** Generates a fresh P-256 ECDH keypair for one hand, exporting the public JWK. */
export async function genHandKeyPair(): Promise<HandKeyPair> {
  const pair = await crypto.subtle.generateKey(
    { name: "ECDH", namedCurve: "P-256" },
    true,
    ["deriveKey"],
  );
  const publicJwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  return { publicJwk, privateKey: pair.privateKey };
}

async function deriveAesKey(privateKey: CryptoKey, peerPublicJwk: JsonWebKey): Promise<CryptoKey> {
  const peerPublic = await crypto.subtle.importKey(
    "jwk",
    peerPublicJwk,
    { name: "ECDH", namedCurve: "P-256" },
    false,
    [],
  );
  return crypto.subtle.deriveKey(
    { name: "ECDH", public: peerPublic },
    privateKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

/** Encrypts a hole-card envelope (any JSON string) to the recipient's per-hand key. */
export async function sealHoleCards(
  dealerPrivate: CryptoKey,
  recipientPublicJwk: JsonWebKey,
  plaintext: string,
): Promise<{ iv: string; ct: string }> {
  const key = await deriveAesKey(dealerPrivate, recipientPublicJwk);
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const enc = new TextEncoder().encode(plaintext);
  const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, enc);
  return { iv: toHex(iv), ct: toHex(new Uint8Array(ct)) };
}

/** Opens a sealed hole-card envelope with the recipient's per-hand private key. */
export async function openHoleCards(
  recipientPrivate: CryptoKey,
  dealerPublicJwk: JsonWebKey,
  sealed: { iv: string; ct: string },
): Promise<string> {
  const key = await deriveAesKey(recipientPrivate, dealerPublicJwk);
  const pt = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: fromHex(sealed.iv) as unknown as BufferSource },
    key,
    fromHex(sealed.ct) as unknown as BufferSource,
  );
  return new TextDecoder().decode(pt);
}

// ---- Coordinator-side round state machine ------------------------------------

/**
 * Tracks one hand's commit/reveal round on the coordinator. The coordinator
 * collects commits from all participants, then reveals, verifying each opening
 * against its commit. A participant who never reveals is dropped via
 * excludeMissing and the round restarts (the caller re-issues commit_req).
 */
export class SeedRound {
  readonly hand: number;
  private participants: string[];
  private commits = new Map<string, string>();
  private shares = new Map<string, Uint8Array>();

  constructor(hand: number, participants: string[]) {
    this.hand = hand;
    this.participants = [...participants].sort();
  }

  order(): string[] {
    return [...this.participants];
  }

  addCommit(from: string, commitHex: string): void {
    if (this.participants.includes(from)) this.commits.set(from, commitHex);
  }

  allCommitted(): boolean {
    return this.participants.every((p) => this.commits.has(p));
  }

  /** Records a reveal; returns "ok", "bad" (commit mismatch), or "unknown". */
  async addReveal(from: string, shareHex: string): Promise<"ok" | "bad" | "unknown"> {
    const commitHex = this.commits.get(from);
    if (!commitHex) return "unknown";
    const share = fromHex(shareHex);
    if (!(await verifyCommit(share, commitHex))) return "bad";
    this.shares.set(from, share);
    return "ok";
  }

  allRevealed(): boolean {
    return this.participants.every((p) => this.shares.has(p));
  }

  /** Drops participants who have not revealed; true if any were removed. */
  excludeMissing(): boolean {
    const kept = this.participants.filter((p) => this.shares.has(p));
    if (kept.length === this.participants.length) return false;
    this.participants = kept;
    this.commits.clear();
    this.shares.clear();
    return true;
  }

  revealedSharesHex(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const p of this.participants) out[p] = toHex(this.shares.get(p)!);
    return out;
  }

  /** The combined seed once every participant has revealed. */
  async seed(): Promise<string> {
    return combineSeed(this.participants.map((p) => this.shares.get(p)!));
  }
}
