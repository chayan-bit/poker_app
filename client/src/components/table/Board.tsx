// The community board: five persistent card slots on a recessed tray. As the
// server reveals streets, new cards deal in via the `deal` primitive rather
// than remounting. Empty slots stay in the DOM so the row never reflows.

import { memo, useEffect, useRef } from "react";
import type { Card as CardT } from "@/net/protocol";
import { Card } from "./Card";
import { deal } from "@/motion/primitives";

interface Props {
  board: CardT[];
}

function BoardImpl({ board }: Props) {
  const refs = useRef<(HTMLDivElement | null)[]>([]);
  const prevCount = useRef(0);

  useEffect(() => {
    for (let i = prevCount.current; i < board.length; i++) {
      const el = refs.current[i];
      if (el) deal(el, { x: -46, y: -70 }, (i - prevCount.current) * 70);
    }
    prevCount.current = board.length;
  }, [board.length]);

  return (
    <div
      className="flex gap-1.5 rounded-2xl px-2.5 py-2"
      style={{
        background: "rgba(0,0,0,0.22)",
        boxShadow: "inset 0 1px 3px rgba(0,0,0,0.45)",
      }}
      aria-label="community cards"
    >
      {[0, 1, 2, 3, 4].map((i) => (
        <div
          key={i}
          ref={(el) => {
            refs.current[i] = el;
          }}
          className="grid place-items-center"
          style={{ width: 50, height: 70 }}
        >
          {board[i] ? (
            <Card card={board[i]} size="md" />
          ) : (
            <div
              className="h-full w-full rounded-lg"
              style={{ border: "1px dashed rgba(255,255,255,0.08)" }}
            />
          )}
        </div>
      ))}
    </div>
  );
}

export const Board = memo(BoardImpl);
