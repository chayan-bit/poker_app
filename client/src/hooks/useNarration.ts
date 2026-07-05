// Screen-reader narration is driven from the SAME event stream the animations
// consume, so audio narration can never drift from the visuals. We subscribe to
// the store's discrete-state changes and push a short phrase into an aria-live
// region. This hook returns the current phrase; render it in a visually-hidden
// live region.

import { useEffect, useRef, useState } from "react";
import { useGame } from "@/store/gameStore";
import { formatChips } from "@/lib/format";

export function useNarration(): string {
  const [phrase, setPhrase] = useState("");
  const prev = useRef({ street: null as string | null, pot: 0, nextToAct: -2 });

  useEffect(() => {
    return useGame.subscribe((s) => {
      const p = prev.current;
      if (s.street && s.street !== p.street) {
        setPhrase(
          s.street === "showdown"
            ? "Showdown."
            : `${cap(s.street)}. Pot ${formatChips(s.pot)}.`,
        );
      } else if (s.nextToAct !== p.nextToAct && s.nextToAct === s.yourSeat) {
        setPhrase(`Your turn. Pot ${formatChips(s.pot)}.`);
      }
      prev.current = {
        street: s.street,
        pot: s.pot,
        nextToAct: s.nextToAct,
      };
    });
  }, []);

  return phrase;
}

function cap(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
