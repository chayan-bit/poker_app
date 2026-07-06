// Fairness screen. Plainly explains the provably-fair commit-reveal shuffle and
// lets a player paste a revealed seed + commitment to verify a past hand
// entirely client-side (SHA-256 in the browser).

import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card, Button, Field, Input } from "@/components/ui/kit";
import { verifyCommitment } from "@/lib/sha";
import { FAIR_FIXTURE } from "@/net/mockServer";

export default function FairnessVerifier() {
  const nav = useNavigate();
  const [seed, setSeed] = useState("");
  const [commitment, setCommitment] = useState("");
  const [result, setResult] = useState<
    { ok: boolean; computed: string } | null
  >(null);
  const [busy, setBusy] = useState(false);

  const run = async () => {
    setBusy(true);
    const r = await verifyCommitment(seed, commitment);
    setResult(r);
    setBusy(false);
  };

  return (
    <Screen title="Provably fair" back={<Back nav={nav} />}>
      <Card>
        <p className="text-sm leading-relaxed text-ink-dim">
          Before each hand the server publishes a{" "}
          <strong className="text-ink">commitment</strong> — the SHA-256 hash of
          a secret shuffle seed. It can't change the deck after seeing anyone's
          cards without breaking that hash. After the hand it{" "}
          <strong className="text-ink">reveals the seed</strong>. Anyone can hash
          the seed and check it matches the commitment. That check runs here, in
          your browser — nothing is sent anywhere.
        </p>
      </Card>

      <Card>
        <div className="flex flex-col gap-4">
          <Field label="Revealed seed">
            <Input
              placeholder="seed from the completed hand"
              value={seed}
              className="mono text-sm"
              onChange={(e) => {
                setSeed(e.target.value);
                setResult(null);
              }}
            />
          </Field>
          <Field label="Published commitment (SHA-256)">
            <Input
              placeholder="64-char hex"
              value={commitment}
              className="mono text-sm"
              onChange={(e) => {
                setCommitment(e.target.value);
                setResult(null);
              }}
            />
          </Field>
          <div className="flex gap-2">
            <Button disabled={!seed || !commitment || busy} onClick={run}>
              {busy ? "Hashing…" : "Verify"}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setSeed(FAIR_FIXTURE.seed);
                setCommitment(FAIR_FIXTURE.commitment);
                setResult(null);
              }}
            >
              Use demo hand
            </Button>
          </div>

          {result && (
            <div
              className="rounded-xl p-3 text-sm"
              style={{
                background: result.ok
                  ? "rgba(63,178,127,0.14)"
                  : "rgba(232,92,92,0.14)",
                color: result.ok ? "#3fb27f" : "var(--danger)",
              }}
            >
              <p className="font-semibold">
                {result.ok ? "Verified — the deck was committed before the deal." : "Mismatch — this seed does not match the commitment."}
              </p>
              <p className="mono mt-1 break-all text-xs opacity-80">
                SHA-256(seed) = {result.computed}
              </p>
            </div>
          )}
        </div>
      </Card>
    </Screen>
  );
}

function Back({ nav }: { nav: (n: number) => void }) {
  return (
    <button
      onClick={() => nav(-1)}
      className="grid h-9 w-9 place-items-center rounded-lg border border-line text-ink-dim"
      aria-label="Back"
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <path d="M15 5l-7 7 7 7" />
      </svg>
    </button>
  );
}
