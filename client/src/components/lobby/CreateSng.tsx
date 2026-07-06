// Create-SNG flow, mirroring the existing CreateRoom screen pattern. Posts to
// POST /api/sng (name, seats, buyIn - the server fills starting stack, blind
// schedule and payouts via tourney.DefaultConfig; there is no client-settable
// "blind speed" knob on the wire, so the default schedule is shown as a
// read-only preview instead of a field).

import { useState } from "react";
import { Card, Button, Field, Input } from "@/components/ui/kit";
import { createSNG, ApiError } from "@/net/api";

export default function CreateSng({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [seats, setSeats] = useState(6);
  const [buyIn, setBuyIn] = useState(100);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setError(null);
    setBusy(true);
    try {
      await createSNG({
        name: name.trim() || "Sit & Go",
        seats,
        buyIn,
      });
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create the sit-and-go.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Create sit &amp; go</h2>
        <button onClick={onClose} className="text-ink-dim" aria-label="Close">
          <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden>
            <path d="M6 6l12 12M18 6 6 18" />
          </svg>
        </button>
      </div>
      <div className="flex flex-col gap-4">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Sit & Go" />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Seats (2-9)">
            <Input
              type="number"
              min={2}
              max={9}
              value={seats}
              onChange={(e) => setSeats(Math.min(9, Math.max(2, +e.target.value)))}
            />
          </Field>
          <Field label="Buy-in (chips)">
            <Input
              type="number"
              min={1}
              value={buyIn}
              onChange={(e) => setBuyIn(Math.max(1, Math.round(+e.target.value)))}
            />
          </Field>
        </div>

        <div className="rounded-xl border border-line p-3 text-sm text-ink-dim">
          <p className="mb-1 font-semibold text-ink-dim">Blind schedule</p>
          <p className="num">10/20 rising every 5 min, standard SNG ramp.</p>
        </div>

        {error && (
          <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
            {error}
          </p>
        )}

        <Button variant="gold" disabled={busy} onClick={submit}>
          {busy ? "Creating..." : "Create sit & go"}
        </Button>
      </div>
    </Card>
  );
}
