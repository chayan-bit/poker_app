// Table menu: the gear icon (kit.tsx Icon "gear") in the top HUD opens a small
// dropdown with Sit out / Sit in and a clearly labeled Leave table action - the
// only way out of the table inside the iOS shell, which has no back button.
// Quiet chrome, never a modal over the felt - a simple anchored popover that
// closes on outside click or selection.
//
// This file also hosts the table-level error toast: gameStore writes lastError
// on any server rejection (and the WS client writes terminal auth/protocol
// errors here too), and this is where it is finally surfaced to the player.

import { memo, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useGame } from "@/store/gameStore";
import { Cmd } from "@/net/protocol";
import { Icon } from "@/components/ui/kit";

/** Error codes that mean the session cannot continue without a fresh load
 *  (expired auth, a server-side protocol bump, or a misconfigured build). These
 *  render as a persistent "Reload" banner instead of a transient toast. */
const REFRESH_CODES = new Set([
  "auth_rejected",
  "protocol_version",
  "misconfigured_api_url",
]);

/** Releases the seat server-side, then tears down the local session. */
function leaveTable(): void {
  const { transport, tableId, disconnect } = useGame.getState();
  if (transport && tableId) {
    transport.send({ type: Cmd.Leave, data: { tableId } });
  }
  disconnect();
}

function TableMenuImpl() {
  const [open, setOpen] = useState(false);
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const nav = useNavigate();

  const yourSeat = useGame((s) => s.yourSeat);
  const seats = useGame((s) => s.seats);
  const handRunning = useGame((s) => s.handRunning);
  const sitOut = useGame((s) => s.sitOut);
  const sitIn = useGame((s) => s.sitIn);

  const hero = seats.find((s) => s.seat === yourSeat);
  const sittingOut = hero?.sittingOut ?? false;

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setConfirmingLeave(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const doLeave = () => {
    leaveTable();
    setOpen(false);
    setConfirmingLeave(false);
    nav("/lobby");
  };

  const onLeaveClick = () => {
    // Guard leaving a live hand behind a confirm so a stray tap does not fold
    // and forfeit chips; leave immediately when no hand is running.
    if (handRunning) setConfirmingLeave(true);
    else doLeave();
  };

  return (
    <>
      <TableErrorToast />
      {yourSeat !== null && (
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
              className="absolute right-0 top-11 z-30 w-52 overflow-hidden rounded-xl"
              style={{
                background: "var(--surface-3)",
                boxShadow: "var(--shadow-2), inset 0 0 0 1px var(--line-hi)",
              }}
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
              <div style={{ height: 1, background: "var(--line)" }} />
              {confirmingLeave ? (
                <MenuItem
                  label="Leave now - forfeit this hand"
                  danger
                  onClick={doLeave}
                />
              ) : (
                <MenuItem label="Leave table" danger onClick={onLeaveClick} />
              )}
            </div>
          )}
        </div>
      )}
    </>
  );
}

function MenuItem({
  label,
  onClick,
  danger,
}: {
  label: string;
  onClick: () => void;
  danger?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      role="menuitem"
      className="no-tap-highlight block w-full px-4 py-3 text-left text-sm"
      style={{ color: danger ? "var(--danger)" : "var(--ink)" }}
    >
      {label}
    </button>
  );
}

// The single surfacing point for lastError. Transient server rejections fade
// after a few seconds; session-fatal errors (expired auth, protocol bump)
// persist with a Reload action, because the table cannot recover on its own.
function TableErrorToast() {
  const lastError = useGame((s) => s.lastError);
  const clearError = useGame((s) => s.clearError);
  const isFatal = lastError ? REFRESH_CODES.has(lastError.code) : false;

  useEffect(() => {
    if (!lastError || isFatal) return;
    const t = window.setTimeout(clearError, 4000);
    return () => window.clearTimeout(t);
  }, [lastError, isFatal, clearError]);

  if (!lastError) return null;

  return (
    <div
      role="alert"
      aria-live="assertive"
      className="fixed left-1/2 z-40 flex w-[min(92vw,26rem)] -translate-x-1/2 items-center gap-3 rounded-xl px-4 py-3 text-sm"
      style={{
        top: "calc(env(safe-area-inset-top) + 2.75rem)",
        background: isFatal ? "rgba(232,180,76,0.16)" : "rgba(232,92,92,0.16)",
        color: isFatal ? "var(--gold)" : "var(--danger)",
        boxShadow: "var(--shadow-2), inset 0 0 0 1px rgba(255,255,255,0.08)",
        backdropFilter: "blur(8px)",
      }}
    >
      <span className="flex-1 font-medium">{lastError.message}</span>
      {isFatal ? (
        <button
          onClick={() => window.location.reload()}
          className="no-tap-highlight shrink-0 rounded-lg px-3 py-1.5 text-xs font-semibold"
          style={{ background: "var(--gold)", color: "#231704" }}
        >
          Reload
        </button>
      ) : (
        <button
          onClick={clearError}
          aria-label="Dismiss"
          className="no-tap-highlight grid h-6 w-6 shrink-0 place-items-center rounded-full"
          style={{ color: "currentColor" }}
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
          >
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </button>
      )}
    </div>
  );
}

export const TableMenu = memo(TableMenuImpl);
