// End-of-session summary: hands played, biggest pot, and each player's net.
// Screenshot-worthy within the existing tokens. Net is chips relative to the
// initial buy-in; free rebuys are not deducted (offline chips are for fun only).

import { motion } from "framer-motion";
import { Button, Card } from "@/components/ui/kit";
import { useNearby } from "./nearbyStore";

const rise = (d: number) => ({
  initial: { opacity: 0, y: 14 },
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0.45, delay: d },
});

export default function SessionSummary({ onDone }: { onDone: () => void }) {
  const summary = useNearby((s) => s.summary);
  const reset = useNearby((s) => s.reset);

  if (!summary) return null;

  return (
    <div className="flex flex-col gap-5">
      <motion.div className="text-center" {...rise(0)}>
        <p className="mono text-xs font-medium uppercase tracking-[0.3em] text-ink-faint">Session complete</p>
        <h1 className="display mt-2 text-4xl">
          Good <span className="display-accent">game</span>
        </h1>
      </motion.div>

      <motion.div {...rise(0.08)}>
        <Card>
          <div className="grid grid-cols-2 gap-3">
            <Stat label="Hands played" value={`${summary.handsPlayed}`} />
            <Stat label="Biggest pot" value={`${summary.biggestPot}`} />
          </div>
        </Card>
      </motion.div>

      <motion.div {...rise(0.16)}>
        <Card>
          <div className="flex flex-col">
            <div className="mb-1 flex items-baseline justify-between text-xs uppercase tracking-[0.18em] text-ink-faint">
              <span>Player</span>
              <span>Net</span>
            </div>
            {summary.rows.map((r) => (
              <div key={r.playerId} className="flex items-center justify-between border-t border-line py-2.5">
                <div className="flex flex-col">
                  <span className="text-base text-ink">{r.name}</span>
                  <span className="num text-xs text-ink-faint">
                    {r.finalStack} chips · bought in {r.buyIn}
                  </span>
                </div>
                <span
                  className="num text-lg font-semibold"
                  style={{ color: r.net > 0 ? "var(--gold)" : r.net < 0 ? "var(--ink-dim)" : "var(--ink)" }}
                >
                  {r.net > 0 ? "+" : ""}
                  {r.net}
                </span>
              </div>
            ))}
          </div>
        </Card>
      </motion.div>

      <motion.div className="flex gap-3" {...rise(0.24)}>
        <Button variant="ghost" className="flex-1" onClick={onDone}>
          Home
        </Button>
        <Button variant="gold" className="flex-1" onClick={() => reset()}>
          Play again
        </Button>
      </motion.div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1 rounded-xl p-3" style={{ background: "var(--surface-3)" }}>
      <span className="text-xs uppercase tracking-[0.16em] text-ink-faint">{label}</span>
      <span className="num display text-3xl">{value}</span>
    </div>
  );
}
