// Join a nearby table. Two-step serverless signaling: scan (or paste) the host's
// invite, get a reply code back to show the host, and once the data channel opens
// the mesh catches you up and seats you. No server. Every async step is guarded
// so a bad code, an unsupported WebAssembly runtime, or a failed connection
// surfaces a clear, retryable error instead of wedging the screen.

import { useRef, useState } from "react";
import { motion } from "framer-motion";
import { Button, Card, Field, Input } from "@/components/ui/kit";
import { QrCode } from "./QrCode";
import { QrScanner } from "./QrScanner";
import { NearbySession } from "./session";
import { useNearby } from "./nearbyStore";
import { errorMessage } from "./errors";

const rise = { initial: { opacity: 0, y: 12 }, animate: { opacity: 1, y: 0 }, transition: { duration: 0.4 } };

export default function JoinFlow({ onReady }: { onReady: (s: NearbySession) => void }) {
  const setPhase = useNearby((s) => s.setPhase);

  const [name, setName] = useState("");
  const [answer, setAnswer] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sessionRef = useRef<NearbySession | null>(null);

  async function join(offerBlob: string) {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const { session, answerBlob } = await NearbySession.join(name.trim() || "Guest", offerBlob);
      sessionRef.current = session;
      onReady(session);
      setAnswer(answerBlob);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (answer) {
    return (
      <motion.div {...rise} className="flex flex-col gap-4">
        <Card>
          <div className="flex flex-col gap-4">
            <div>
              <h2 className="display text-2xl">Almost in</h2>
              <p className="mt-1 text-sm text-ink-dim">
                Show this reply code to your host. Once they scan it, you are seated
                automatically.
              </p>
            </div>
            <QrCode label="Your reply code" value={answer} hint="Let the host scan or paste this." />
          </div>
        </Card>
        <Button variant="gold" onClick={() => setPhase("table")}>
          Go to the table
        </Button>
      </motion.div>
    );
  }

  return (
    <motion.div {...rise}>
      <Card>
        <div className="flex flex-col gap-5">
          <div>
            <h2 className="display text-2xl">Join a table</h2>
            <p className="mt-1 text-sm text-ink-dim">
              Enter your name, then scan the invite your host is showing (or paste it).
            </p>
          </div>
          <Field label="Your name">
            <Input autoFocus placeholder="e.g. River" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <QrScanner
            label="Host's invite"
            hint="Chips here are session-only and never touch your online balance."
            cta="Join table"
            busy={busy}
            onResult={join}
          />
          {error && (
            <p className="rounded-xl p-3 text-xs" style={{ background: "var(--surface-3)", color: "var(--danger, #c0392b)" }}>
              {error}
            </p>
          )}
        </div>
      </Card>
    </motion.div>
  );
}
