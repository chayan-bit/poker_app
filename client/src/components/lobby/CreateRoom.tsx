// Private "Create room": host sets blinds, timer length, buy-in range, max
// seats (up to 10) and rule toggles (run-it-twice, bomb-pot frequency,
// straddles, dealer's-choice rotation).

import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, Button, Field, Input, Toggle } from "@/components/ui/kit";

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

  return (
    <Card>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Create private room</h2>
        <button onClick={onClose} className="text-ink-dim" aria-label="Close">
          ✕
        </button>
      </div>
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
          <Field label="Max seats (≤10)">
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

        <Button
          variant="gold"
          onClick={() => {
            const code = Math.random().toString(36).slice(2, 8).toUpperCase();
            nav(`/table?join=${code}`);
          }}
        >
          Create & take seat
        </Button>
      </div>
    </Card>
  );
}
