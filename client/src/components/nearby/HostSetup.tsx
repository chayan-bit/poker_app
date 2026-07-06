// Host a nearby table: pick a name, stakes, and starting stack, then start your
// own peer-to-peer network and invite friends by sharing a connection blob. The
// server is never contacted. Chips are session-scoped and never touch online
// balances - stated plainly here.

import { useRef, useState } from "react";
import { motion } from "framer-motion";
import { Button, Card, Field, Input } from "@/components/ui/kit";
import { QrCode } from "./QrCode";
import { QrScanner } from "./QrScanner";
import { NearbySession, type HostInvite } from "./session";
import { useNearby } from "./nearbyStore";
import { errorMessage } from "./errors";

const STAKES = [
  { sb: 1, bb: 2 },
  { sb: 2, bb: 5 },
  { sb: 5, bb: 10 },
];
const STACKS = [100, 200, 500];
const rise = { initial: { opacity: 0, y: 12 }, animate: { opacity: 1, y: 0 }, transition: { duration: 0.4 } };

export default function HostSetup({ onReady }: { onReady: (s: NearbySession) => void }) {
  const setConfig = useNearby((s) => s.setConfig);
  const setPhase = useNearby((s) => s.setPhase);

  const [name, setName] = useState("");
  const [tableName, setTableName] = useState("Kitchen table");
  const [stakes, setStakes] = useState(0);
  const [stack, setStack] = useState(1);
  const [step, setStep] = useState<"form" | "invite">("form");
  const [invite, setInvite] = useState<HostInvite | null>(null);
  const [joined, setJoined] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sessionRef = useRef<NearbySession | null>(null);

  async function createTable() {
    if (busy) return;
    setBusy(true);
    setError(null);
    const cfg = {
      tableName: tableName.trim() || "Kitchen table",
      smallBlind: STAKES[stakes].sb,
      bigBlind: STAKES[stakes].bb,
      startingStack: STACKS[stack],
    };
    try {
      setConfig(cfg);
      const session = await NearbySession.host(cfg, name.trim() || "Host");
      sessionRef.current = session;
      onReady(session);
      setInvite(await session.createInvite());
      setStep("invite");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function acceptAnswer(answer: string) {
    if (!invite || !sessionRef.current || busy) return;
    setBusy(true);
    setError(null);
    try {
      await invite.accept(answer);
      setJoined((n) => n + 1);
      setInvite(await sessionRef.current.createInvite());
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (step === "invite") {
    return (
      <motion.div {...rise} className="flex flex-col gap-4">
        <Card>
          <div className="flex flex-col gap-4">
            <div>
              <h2 className="display text-2xl">Invite a friend</h2>
              <p className="mt-1 text-sm text-ink-dim">
                Share this code with a friend on the same network. They paste it in,
                send you a code back, and you paste that below.
              </p>
            </div>
            {invite && (
              <QrCode
                label="Your invite code"
                value={invite.offerBlob}
                hint="Your friend scans this on their Join screen."
              />
            )}
            <QrScanner
              label="Their reply code"
              hint="One code per friend. Repeat to seat more players."
              cta="Seat this friend"
              busy={busy}
              onResult={acceptAnswer}
            />
            {error && (
              <p className="rounded-xl p-3 text-xs" style={{ background: "var(--surface-3)", color: "var(--danger, #c0392b)" }}>
                {error}
              </p>
            )}
            {joined > 0 && (
              <p className="num text-sm text-ink-dim">
                {joined} {joined === 1 ? "friend" : "friends"} seated.
              </p>
            )}
          </div>
        </Card>
        <Button variant="gold" disabled={joined < 1} onClick={() => setPhase("table")}>
          {joined < 1 ? "Seat at least one friend" : "Go to the table"}
        </Button>
      </motion.div>
    );
  }

  return (
    <motion.div {...rise}>
      <Card>
        <div className="flex flex-col gap-5">
          <div>
            <h2 className="display text-2xl">Host a table</h2>
            <p className="mt-1 text-sm text-ink-dim">
              Starts your own network on this device. No server, no sign-in.
            </p>
          </div>
          <Field label="Your name">
            <Input autoFocus placeholder="e.g. Ace" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label="Table name">
            <Input value={tableName} onChange={(e) => setTableName(e.target.value)} />
          </Field>
          <Picker
            label="Stakes"
            options={STAKES.map((s) => `${s.sb} / ${s.bb}`)}
            selected={stakes}
            onSelect={setStakes}
          />
          <Picker
            label="Starting stack"
            options={STACKS.map((s) => `${s}`)}
            selected={stack}
            onSelect={setStack}
          />
          <p
            className="rounded-xl p-3 text-xs text-ink-dim"
            style={{ background: "var(--surface-3)", boxShadow: "inset 0 0 0 1px var(--line)" }}
          >
            These chips are for this table only. They are never synced to your
            online balance, and rebuys are free.
          </p>
          {error && (
            <p className="rounded-xl p-3 text-xs" style={{ background: "var(--surface-3)", color: "var(--danger, #c0392b)" }}>
              {error}
            </p>
          )}
          <Button variant="gold" disabled={!name.trim() || busy} onClick={createTable}>
            {busy ? "Starting…" : "Start table"}
          </Button>
        </div>
      </Card>
    </motion.div>
  );
}

function Picker({
  label,
  options,
  selected,
  onSelect,
}: {
  label: string;
  options: string[];
  selected: number;
  onSelect: (i: number) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-sm text-ink-dim">{label}</span>
      <div className="grid grid-cols-3 gap-2">
        {options.map((opt, i) => {
          const on = i === selected;
          return (
            <button
              key={opt}
              onClick={() => onSelect(i)}
              className="num no-tap-highlight min-h-[46px] rounded-xl text-base font-semibold transition-transform duration-150 active:scale-[0.98]"
              style={{
                background: on ? "var(--action-blue)" : "var(--surface-4)",
                color: on ? "#fff" : "var(--ink-dim)",
                boxShadow: on ? "var(--shadow-1)" : "inset 0 0 0 1px var(--line-hi)",
              }}
            >
              {opt}
            </button>
          );
        })}
      </div>
    </div>
  );
}
