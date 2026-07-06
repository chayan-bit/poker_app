// Table menu: the gear icon (kit.tsx Icon "gear") in the top HUD opens a small
// dropdown with Sit out / Sit in. Quiet chrome, never a modal over the felt -
// it's a simple anchored popover that closes on outside click or selection.

import { memo, useEffect, useRef, useState } from "react";
import { useGame } from "@/store/gameStore";
import { Icon } from "@/components/ui/kit";

function TableMenuImpl() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const yourSeat = useGame((s) => s.yourSeat);
  const seats = useGame((s) => s.seats);
  const sitOut = useGame((s) => s.sitOut);
  const sitIn = useGame((s) => s.sitIn);

  const hero = seats.find((s) => s.seat === yourSeat);
  const sittingOut = hero?.sittingOut ?? false;

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  if (yourSeat === null) return null;

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="glass grid h-9 w-9 place-items-center rounded-full text-ink-dim no-tap-highlight"
        aria-label="Table menu"
        aria-expanded={open}
      >
        <Icon name="gear" size={18} />
      </button>
      {open && (
        <div
          className="absolute right-0 top-11 z-30 w-44 overflow-hidden rounded-xl"
          style={{ background: "var(--surface-3)", boxShadow: "var(--shadow-2), inset 0 0 0 1px var(--line-hi)" }}
          role="menu"
        >
          {sittingOut ? (
            <MenuItem
              label="Sit in"
              onClick={() => {
                sitIn();
                setOpen(false);
              }}
            />
          ) : (
            <MenuItem
              label="Sit out"
              onClick={() => {
                sitOut();
                setOpen(false);
              }}
            />
          )}
        </div>
      )}
    </div>
  );
}

function MenuItem({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      role="menuitem"
      className="no-tap-highlight block w-full px-4 py-3 text-left text-sm text-ink"
    >
      {label}
    </button>
  );
}

export const TableMenu = memo(TableMenuImpl);
