// Quiet Rebuy entry point in the bottom band: appears only when the hero's own
// seat is broke (stack 0) and no hand is in flight. Opens the RebuySheet.

import { memo, useState } from "react";
import { useGame } from "@/store/gameStore";
import { RebuySheet } from "./RebuySheet";

function RebuyButtonImpl() {
  const [open, setOpen] = useState(false);
  const yourSeat = useGame((s) => s.yourSeat);
  const seats = useGame((s) => s.seats);
  const handRunning = useGame((s) => s.handRunning);

  const hero = seats.find((s) => s.seat === yourSeat);
  const eligible = !!hero && hero.stack === 0 && !handRunning;

  if (!eligible) return null;

  return (
    <>
      <div className="flex justify-center px-4 pb-2">
        <button
          onClick={() => setOpen(true)}
          className="no-tap-highlight min-h-[44px] rounded-xl px-5 text-sm font-semibold tracking-tight"
          style={{
            background: "var(--surface-4)",
            color: "var(--ink)",
            boxShadow: "var(--shadow-1), inset 0 0 0 1px var(--line-hi)",
          }}
        >
          Rebuy
        </button>
      </div>
      <RebuySheet open={open} onClose={() => setOpen(false)} />
    </>
  );
}

export const RebuyButton = memo(RebuyButtonImpl);
