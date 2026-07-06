// Private "Create room": host sets blinds and max seats, then the server mints
// a table + 6-char join code (POST /api/rooms). On success we show the code and
// share URL, and offer to take the seat.
//
// Wire note: the room protocol (CreateRoomRequest) accepts only smallBlind,
// bigBlind, maxSeats and visibility. The advanced host preferences below
// (action timer, buy-in range, run-it-twice / straddles / dealer's-choice /
// bomb-pot) are NOT yet part of the create-room wire contract, so they are kept
// as local host intent and are not transmitted. Surfacing them here keeps the
// host UX intact; wiring them is a server-side seam (see the change report).

import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, Button, Field, Input, Toggle } from "@/components/ui/kit";
import { createRoom, ApiError, type CreateRoomResponse } from "@/net/api";

export default function CreateRoom({ onClose }: { onClose: () => void }) {
  const nav = useNavigate();
  const [sb, setSb] = useState(1);
  const [bb, setBb] = useState(2);
  const [timer, setTimer] = useState(20);
  const [minBuy, setMinBuy] = useState(40);
  const [maxBuy, setMaxBuy] = useState(200);
  const [maxSeats, setMaxSeats] = useState(6);
  const [runItTwice, setRunItTwice] = useState(true);
  const [bombPot, setBombPot] = useState(0); // every N hands, 0 = off
  const [straddles, setStraddles] = useState(false);
  const [dealersChoice, setDealersChoice] = useState(false);

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreateRoomResponse | null>(null);
  const [copied, setCopied] = useState(false);

  const create = async () => {
    setError(null);
    if (bb <= sb) {
      setError("Big blind must be greater than the small blind.");
      return;
    }
    if (maxSeats < 2 || maxSeats > 10) {
      setError("Max seats must be between 2 and 10.");
      return;
    }
    setBusy(true);
    try {
      const room = await createRoom({
        smallBlind: sb,
        bigBlind: bb,
        maxSeats,
        visibility: "private",
      });
      setCreated(room);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? `${err.message} (${err.code})`
          : "Could not create the room. Please try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  const copyCode = async () => {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.joinCode);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard can be blocked (permissions/insecure context); the code is
      // still visible on screen, so this is a soft failure.
      setCopied(false);
    }
  };

  return (
    <Card>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Create private room</h2>
        <button onClick={onClose} className="text-ink-dim" aria-label="Close">
          <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden>
            <path d="M6 6l12 12M18 6 6 18" />
          </svg>
        </button>
      </div>

      {created ? (
        <div className="flex flex-col gap-4">
          <div className="rounded-xl border border-line p-3">
            <p className="text-sm text-ink-dim">Room ready. Share this code:</p>
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="mono text-2xl font-semibold uppercase tracking-[0.2em]">
                {created.joinCode}
              </span>
              <button
                onClick={() => void copyCode()}
                className="rounded-lg border border-line px-3 py-1.5 text-xs font-semibold text-ink-dim"
              >
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
            <p className="num mt-2 text-xs text-ink-faint break-all">{created.joinUrl}</p>
          </div>
          <Button
            variant="gold"
            onClick={() => nav(`/table?join=${encodeURIComponent(created.tableId)}`)}
          >
            Take seat
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Small blind">
              <Input type="number" min={1} value={sb} onChange={(e) => setSb(+e.target.value)} />
            </Field>
            <Field label="Big blind">
              <Input type="number" min={1} value={bb} onChange={(e) => setBb(+e.target.value)} />
            </Field>
            <Field label="Action timer (s)">
              <Input type="number" min={5} value={timer} onChange={(e) => setTimer(+e.target.value)} />
            </Field>
            <Field label="Max seats (up to 10)">
              <Input
                type="number"
                min={2}
                max={10}
                value={maxSeats}
                onChange={(e) => setMaxSeats(Math.min(10, +e.target.value))}
              />
            </Field>
            <Field label="Min buy-in (BB)">
              <Input type="number" value={minBuy} onChange={(e) => setMinBuy(+e.target.value)} />
            </Field>
            <Field label="Max buy-in (BB)">
              <Input type="number" value={maxBuy} onChange={(e) => setMaxBuy(+e.target.value)} />
            </Field>
          </div>

          <div className="rounded-xl border border-line p-3">
            <p className="mb-1 text-sm font-semibold text-ink-dim">Rules</p>
            <Toggle label="Run it twice" checked={runItTwice} onChange={() => setRunItTwice((v) => !v)} />
            <Toggle label="Straddles allowed" checked={straddles} onChange={() => setStraddles((v) => !v)} />
            <Toggle
              label="Dealer's-choice rotation"
              checked={dealersChoice}
              onChange={() => setDealersChoice((v) => !v)}
            />
            <div className="flex items-center justify-between py-2">
              <span className="text-base">Bomb pot every</span>
              <select
                value={bombPot}
                onChange={(e) => setBombPot(+e.target.value)}
                className="rounded-lg border border-line bg-surface px-2 py-1.5 text-sm"
              >
                <option value={0}>Off</option>
                <option value={10}>10 hands</option>
                <option value={25}>25 hands</option>
                <option value={50}>50 hands</option>
              </select>
            </div>
          </div>

          {error && (
            <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
              {error}
            </p>
          )}

          <Button variant="gold" disabled={busy} onClick={() => void create()}>
            {busy ? "Creating..." : "Create room"}
          </Button>
        </div>
      )}
    </Card>
  );
}
