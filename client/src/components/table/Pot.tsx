// The central pot: a real chip stack plus a tabular gold readout on a frosted
// pill. Always visible without tapping. The node is persistent; the pot-push
// animation targets it from Table.

import { memo, forwardRef } from "react";
import { formatAmount } from "@/lib/format";
import { useSettings } from "@/store/settingsStore";
import { ChipStack } from "./Chips";

interface Props {
  pot: number;
  bb: number;
}

export const Pot = memo(
  forwardRef<HTMLDivElement, Props>(function Pot({ pot, bb }, ref) {
    const showInBB = useSettings((s) => s.showInBB);
    if (pot <= 0) return <div ref={ref} />;
    return (
      <div ref={ref} className="gpu pointer-events-none flex flex-col items-center gap-1">
        <ChipStack amount={pot} size={30} />
        <div className="glass flex items-center gap-1.5 rounded-full px-3 py-1">
          <span className="text-[10px] font-medium uppercase tracking-widest text-ink-faint">
            Pot
          </span>
          <span className="num text-base font-semibold" style={{ color: "var(--gold)" }}>
            {formatAmount(pot, bb, showInBB)}
          </span>
        </div>
      </div>
    );
  }),
);
