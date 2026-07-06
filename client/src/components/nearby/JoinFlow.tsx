// Join a nearby table. Two-step serverless signaling exactly as rtc.ts models
// it: paste the host's invite blob, get a reply blob back to show the host, and
// once the data channel opens the mesh catches you up and seats you. No server.

import { useRef, useState } from "react";
import { motion } from "framer-motion";
import { Button, Card, Field, Input } from "@/components/ui/kit";
import { BlobInput, CodeBlob } from "./CodeBlob";
import { NearbySession } from "./session";
import { useNearby } from "./nearbyStore";

const rise = { initial: { opacity: 0, y: 12 }, animate: { opacity: 1, y: 0 }, transition: { duration: 0.4 } };

export default function JoinFlow({ onReady }: { onReady: (s: NearbySession) => void }) {
  const config = useNearby((s) => s.config);
  const setPhase = useNearby((s) => s.setPhase);

  const [name, setName] = useState("");
  const [answer, setAnswer] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const sessionRef = useRef<NearbySession | null>(null);

  async function join(offerBlob: string) {
    setBusy(true);
    const { session, answerBlob } = await NearbySession.join(config, name.trim() || "Guest", offerBlob);
    sessionRef.current = session;
    onReady(session);
    setAnswer(answerBlob);
    setBusy(false);
  }

  if (answer) {
    return (
      <motion.div {...rise} className="flex flex-col gap-4">
        <Card>
          <div className="flex flex-col gap-4">
            <div>
              <h2 className="display text-2xl">Almost in</h2>
              <p className="mt-1 text-sm text-ink-dim">
                Send this reply code back to your host. Once they paste it, you are
                seated automatically.
              </p>
            </div>
            <CodeBlob label="Your reply code" value={answer} hint="Scan or paste this on the host's screen." />
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
              Enter your name and paste the invite code your host shared.
            </p>
          </div>
          <Field label="Your name">
            <Input autoFocus placeholder="e.g. River" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <BlobInput
            label="Invite code"
            placeholder="Paste the host's invite code"
            hint="Chips here are session-only and never touch your online balance."
            cta="Join table"
            busy={busy}
            onSubmit={join}
          />
        </div>
      </Card>
    </motion.div>
  );
}
