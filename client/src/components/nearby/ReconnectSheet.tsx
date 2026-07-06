// Reconnect overlay (issue #28 hardening): when this peer's link to the table
// drops, its seat is held for a grace window and this sheet re-runs the exact
// invite/answer exchange over a fresh channel. The mesh then heals the log via
// gossip catch-up, so no separate snapshot step is needed. Host and guest see the
// mirror-image flow, reusing the same QR show/scan components as first-time join.

import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import { Button } from "@/components/ui/kit";
import { QrCode } from "./QrCode";
import { QrScanner } from "./QrScanner";
import { errorMessage } from "./errors";
import type { HostInvite, NearbySession } from "./session";

export function ReconnectSheet({ session, onClose }: { session: NearbySession; onClose: () => void }) {
  const isHost = session.isHostRole;
  const [invite, setInvite] = useState<HostInvite | null>(null);
  const [answer, setAnswer] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isHost) return;
    let alive = true;
    void session
      .createInvite()
      .then((inv) => alive && setInvite(inv))
      .catch((err) => alive && setError(errorMessage(err)));
    return () => {
      alive = false;
    };
  }, [isHost, session]);

  async function acceptAnswer(blob: string) {
    if (!invite || busy) return;
    setBusy(true);
    setError(null);
    try {
      await invite.accept(blob);
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function guestReconnect(offerBlob: string) {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      setAnswer(await session.acceptReconnectOffer(offerBlob));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 p-4 sm:items-center">
      <motion.div
        initial={{ opacity: 0, y: 24 }}
        animate={{ opacity: 1, y: 0 }}
        className="w-full max-w-md rounded-2xl p-5"
        style={{ background: "var(--surface-2, var(--surface))", boxShadow: "var(--shadow-2)", maxHeight: "90vh", overflowY: "auto" }}
      >
        <div className="flex flex-col gap-4">
          <div>
            <h2 className="display text-2xl">Reconnect</h2>
            <p className="mt-1 text-sm text-ink-dim">
              Your seat is being held. {isHost
                ? "Send this fresh invite to the friend who dropped and scan their reply."
                : "Scan a fresh invite from your host to rejoin and re-sync."}
            </p>
          </div>

          {isHost && invite && (
            <QrCode label="Fresh invite" value={invite.offerBlob} hint="Have the returning friend scan this." />
          )}
          {isHost && (
            <QrScanner label="Their reply" hint="Paste it if the camera is unavailable." cta="Reconnect" busy={busy} onResult={acceptAnswer} />
          )}

          {!isHost && !answer && (
            <QrScanner label="Host's fresh invite" hint="Paste it if the camera is unavailable." cta="Reconnect" busy={busy} onResult={guestReconnect} />
          )}
          {!isHost && answer && (
            <QrCode label="Your reply code" value={answer} hint="Show this to your host to finish reconnecting." />
          )}

          {error && (
            <p className="rounded-xl p-3 text-xs" style={{ background: "var(--surface-3)", color: "var(--danger, #c0392b)" }}>
              {error}
            </p>
          )}

          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
        </div>
      </motion.div>
    </div>
  );
}
