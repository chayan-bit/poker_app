// Client-side commit-reveal verification. The server publishes commitment =
// SHA-256(seed) before dealing; after the hand it reveals seed. Any client can
// recompute SHA-256(seed) and confirm it equals the commitment.

export async function sha256Hex(input: string): Promise<string> {
  const bytes = new TextEncoder().encode(input);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return [...new Uint8Array(digest)]
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

export interface VerifyResult {
  ok: boolean;
  computed: string;
}

export async function verifyCommitment(
  seed: string,
  commitment: string,
): Promise<VerifyResult> {
  const computed = await sha256Hex(seed);
  return { ok: computed === commitment.trim().toLowerCase(), computed };
}
