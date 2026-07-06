// The nearby table view: the unchanged online Table, fed by the mesh through the
// store bridge, plus nearby-only chrome - the "shuffling together" micro-state,
// the dealer-loss void toast, a reconnecting-peer notice, the dishonest-dealer
// red banner, and an End session control. All animation is transform/opacity.

import { AnimatePresence, motion } from "framer-motion";
import Table from "@/components/table/Table";
import { useNearby } from "./nearbyStore";
import type { NearbySession } from "./session";

const toast = {
  initial: { opacity: 0, y: -12 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -12 },
  transition: { duration: 0.28 },
};

export default function NearbyTable({ session }: { session: NearbySession | null }) {
  const shuffling = useNearby((s) => s.shuffling);
  const voidToast = useNearby((s) => s.voidToast);
  const reconnecting = useNearby((s) => s.reconnecting);
  const dishonestHand = useNearby((s) => s.dishonestHand);
  const setVoidToast = useNearby((s) => s.setVoidToast);
  const setDishonest = useNearby((s) => s.setDishonest);

  return (
    <div className="relative h-full w-full">
      <Table />

      {/* Dishonest-dealer flag: clear red banner naming the offending hand. */}
      <AnimatePresence>
        {dishonestHand && (
          <motion.button
            {...toast}
            onClick={() => setDishonest(null)}
            className="mono absolute left-1/2 top-3 z-30 -translate-x-1/2 rounded-xl px-4 py-2 text-xs font-semibold"
            style={{ background: "var(--danger, #c0392b)", color: "#fff", boxShadow: "var(--shadow-2)" }}
          >
            Fairness alert: hand {dishonestHand} was dealt with an unfair seed. Tap to dismiss.
          </motion.button>
        )}
      </AnimatePresence>

      {/* Stacked transient notices below the danger slot. */}
      <div className="pointer-events-none absolute left-1/2 top-14 z-20 flex -translate-x-1/2 flex-col items-center gap-2">
        <AnimatePresence>
          {shuffling && (
            <motion.div
              key="shuffling"
              {...toast}
              className="rounded-full px-4 py-1.5 text-xs text-ink-dim"
              style={{ background: "var(--surface-3)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
            >
              Shuffling together…
            </motion.div>
          )}
          {voidToast && (
            <motion.button
              key="void"
              {...toast}
              onClick={() => setVoidToast(null)}
              className="pointer-events-auto rounded-full px-4 py-1.5 text-xs font-medium text-ink"
              style={{ background: "var(--surface-4)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
            >
              {voidToast}
            </motion.button>
          )}
          {reconnecting.map((n) => (
            <motion.div
              key={`rc-${n}`}
              {...toast}
              className="rounded-full px-4 py-1.5 text-xs text-ink-dim"
              style={{ background: "var(--surface-3)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
            >
              {n} disconnected - holding their seat…
            </motion.div>
          ))}
        </AnimatePresence>
      </div>

      <button
        onClick={() => session?.end()}
        className="no-tap-highlight absolute right-3 top-3 z-30 rounded-lg px-3 py-1.5 text-xs font-medium text-ink-dim transition-transform duration-150 active:scale-[0.97]"
        style={{ background: "var(--surface-4)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
      >
        End session
      </button>
    </div>
  );
}
