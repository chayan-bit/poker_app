// Lobby. Public: real cash tables from GET /api/tables + sit-and-gos, with a
// one-tap Quick Seat hero (POST /api/quickseat at a chosen stake). Private:
// create room / join by 6-char code (POST /api/rooms, /api/rooms/join). Plus
// the friends panel. Framer Motion is allowed here (non-table chrome).
//
// Every table entry drives the app's existing navigation into the table:
// nav("/table?join=<tableId>"). Every async surface (tables, quickseat, join,
// sng) has loading / empty / error states surfaced inline - never as a modal.

import { lazy, Suspense, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { motion } from "framer-motion";
import { Screen, Card, Button, Input, SpadeMark, Wordmark, Icon } from "@/components/ui/kit";
import { ChipStack } from "@/components/table/Chips";
import { FriendsPanel } from "./FriendsPanel";
import {
  listTables,
  quickseat,
  joinRoom,
  listSNG,
  registerSNG,
  ApiError,
  type PublicTable,
  type SngView,
} from "@/net/api";

const CreateRoom = lazy(() => import("./CreateRoom"));
const CreateSng = lazy(() => import("./CreateSng"));

// The only stakes quickseat will match on (server: lobby.quickseatBlinds).
const QUICK_STAKES = [25, 50, 100, 500, 1000] as const;

type Tier = "micro" | "low" | "mid" | "high";

const TIER_COLOR: Record<Tier, string> = {
  micro: "var(--action-blue)",
  low: "var(--success)",
  mid: "var(--gold)",
  high: "var(--danger)",
};

// Server does not expose a stake "tier"; derive one from the small blind so the
// row accent stays meaningful without fabricating data.
function tierFor(smallBlind: number): Tier {
  if (smallBlind < 50) return "micro";
  if (smallBlind < 500) return "low";
  if (smallBlind < 1000) return "mid";
  return "high";
}

type LoadState = "loading" | "ready" | "error";

const EASE = [0.22, 1, 0.36, 1] as const;
const rise = (delay: number) => ({
  initial: { opacity: 0, y: 12 },
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0.5, ease: EASE, delay },
});

export default function Lobby() {
  const nav = useNavigate();
  const goToTable = (tableId: string) =>
    nav(`/table?join=${encodeURIComponent(tableId)}`);

  // ---- public tables ----
  const [tables, setTables] = useState<PublicTable[]>([]);
  const [tablesState, setTablesState] = useState<LoadState>("loading");
  const [tablesError, setTablesError] = useState<string | null>(null);

  // ---- quick seat ----
  const [quickStake, setQuickStake] = useState<number>(QUICK_STAKES[0]);
  const [seating, setSeating] = useState(false);
  const [seatError, setSeatError] = useState<string | null>(null);

  // ---- private ----
  const [code, setCode] = useState("");
  const [joining, setJoining] = useState(false);
  const [joinError, setJoinError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  // ---- sit-and-go ----
  const [sngs, setSngs] = useState<SngView[]>([]);
  const [sngListError, setSngListError] = useState<string | null>(null);
  const [creatingSng, setCreatingSng] = useState(false);
  const [registering, setRegistering] = useState<string | null>(null);
  const [registerError, setRegisterError] = useState<{ id: string; message: string } | null>(null);

  const refreshTables = () => {
    setTablesState("loading");
    listTables()
      .then((rows) => {
        setTables(rows);
        setTablesState("ready");
      })
      .catch((err) => {
        setTablesError(err instanceof ApiError ? err.message : "Could not load tables.");
        setTablesState("error");
      });
  };

  const refreshSngs = () => {
    listSNG()
      .then((rows) => {
        setSngs(rows);
        setSngListError(null);
      })
      .catch((err) => {
        setSngListError(err instanceof ApiError ? err.message : "Could not load sit-and-gos.");
      });
  };

  useEffect(() => {
    refreshTables();
    refreshSngs();
  }, []);

  const seatMe = async () => {
    setSeatError(null);
    setSeating(true);
    try {
      const res = await quickseat(quickStake);
      goToTable(res.tableId);
    } catch (err) {
      setSeatError(
        err instanceof ApiError
          ? `${err.message} (${err.code})`
          : "Could not find a seat. Please try again.",
      );
    } finally {
      setSeating(false);
    }
  };

  const joinByCode = async () => {
    setJoinError(null);
    setJoining(true);
    try {
      const res = await joinRoom(code);
      goToTable(res.tableId);
    } catch (err) {
      setJoinError(
        err instanceof ApiError && err.code === "not_found"
          ? "No room with that code."
          : err instanceof ApiError
            ? err.message
            : "Could not join the room.",
      );
    } finally {
      setJoining(false);
    }
  };

  const register = async (sngId: string) => {
    setRegisterError(null);
    setRegistering(sngId);
    try {
      await registerSNG(sngId);
      nav("/table");
    } catch (err) {
      const message =
        err instanceof ApiError && err.code === "insufficient_funds"
          ? "Not enough chips for this buy-in."
          : err instanceof ApiError
            ? err.message
            : "Could not register.";
      setRegisterError({ id: sngId, message });
    } finally {
      setRegistering(null);
    }
  };

  return (
    <Screen wide>
      {/* Brand bar */}
      <motion.header className="flex items-center justify-between" {...rise(0)}>
        <button onClick={() => nav("/")} className="flex items-center gap-2.5 no-tap-highlight">
          <span
            className="grid h-9 w-9 place-items-center rounded-xl"
            style={{
              background: "linear-gradient(150deg, var(--felt-hi), var(--felt-edge))",
              boxShadow: "var(--shadow-1)",
              color: "rgba(255,255,255,0.85)",
            }}
          >
            <SpadeMark size={20} />
          </span>
          <Wordmark size={19} />
        </button>
        {/* No chip-balance chip: the server exposes no balance/me endpoint yet
            (economy.Ledger.Balance is internal-only), so a real value is not
            available. We omit it rather than fabricate one. */}
        <div className="flex items-center gap-3">
          <button
            onClick={() => nav("/friends")}
            className="grid h-9 w-9 place-items-center rounded-xl text-ink-dim no-tap-highlight"
            style={{ background: "var(--surface-3)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
            aria-label="Friends"
          >
            <svg width={18} height={18} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round">
              <circle cx={9} cy={8} r={3.2} />
              <path d="M3.5 19c.6-3.1 2.8-4.8 5.5-4.8s4.9 1.7 5.5 4.8" />
              <circle cx={16.8} cy={9.2} r={2.5} />
              <path d="M15.4 14.6c2.3.2 4.2 1.7 4.9 4.2" />
            </svg>
          </button>
          <button
            onClick={() => nav("/settings")}
            className="grid h-9 w-9 place-items-center rounded-xl text-ink-dim no-tap-highlight"
            style={{ background: "var(--surface-3)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
            aria-label="Settings"
          >
            <Icon name="gear" size={18} />
          </button>
        </div>
      </motion.header>

      {/* Quick Seat hero */}
      <motion.div {...rise(0.06)}>
        <div
          className="card-edge relative overflow-hidden rounded-2xl p-5"
          style={{ background: "linear-gradient(135deg, color-mix(in oklab, var(--felt-hi), transparent 82%), var(--surface-2) 60%)" }}
        >
          <div className="absolute -right-6 -top-4 opacity-70">
            <ChipStack amount={2600} size={40} />
          </div>
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-ink-faint">
            Fastest way in
          </p>
          <h2 className="display mt-1 text-[1.65rem]">Quick <span className="display-accent">Seat</span></h2>
          <p className="mt-1 max-w-sm text-sm text-ink-dim">
            We drop you into the best open seat at your stake, instantly.
          </p>

          <div className="mt-3 flex flex-wrap gap-1.5">
            {QUICK_STAKES.map((stake) => {
              const active = stake === quickStake;
              return (
                <button
                  key={stake}
                  onClick={() => setQuickStake(stake)}
                  aria-pressed={active}
                  className="num rounded-full px-3 py-1.5 text-sm font-semibold transition-transform duration-150 hover:-translate-y-[1px] no-tap-highlight"
                  style={{
                    background: active ? "var(--gold)" : "var(--surface-3)",
                    color: active ? "#231704" : "var(--ink-dim)",
                    boxShadow: active ? "var(--shadow-1)" : "inset 0 0 0 1px var(--line-hi)",
                  }}
                >
                  {stake} / {stake * 2}
                </button>
              );
            })}
          </div>

          {seatError && (
            <p className="num mt-3 text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
              {seatError}
            </p>
          )}

          <Button variant="gold" className="mt-4" disabled={seating} onClick={() => void seatMe()}>
            {seating ? "Finding a seat..." : "Seat me now"}
          </Button>
        </div>
      </motion.div>

      <div className="grid grid-cols-1 gap-5 md:grid-cols-[1.3fr_1fr]">
        {/* Public tables */}
        <motion.div className="flex flex-col gap-2.5" {...rise(0.12)}>
          <div className="flex items-center justify-between">
            <SectionLabel>Public tables</SectionLabel>
            <button
              onClick={refreshTables}
              className="text-xs font-semibold uppercase tracking-wider text-ink-dim no-tap-highlight"
            >
              Refresh
            </button>
          </div>

          {tablesState === "loading" && (
            <Card>
              <p className="text-sm text-ink-faint" role="status">Loading tables...</p>
            </Card>
          )}

          {tablesState === "error" && (
            <Card>
              <div className="flex items-center justify-between gap-3">
                <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
                  {tablesError}
                </p>
                <Button variant="ghost" className="min-h-0 px-3 py-1.5 text-sm" onClick={refreshTables}>
                  Retry
                </Button>
              </div>
            </Card>
          )}

          {tablesState === "ready" && tables.length === 0 && (
            <Card>
              <p className="text-sm text-ink-faint">
                No public tables open. Use Quick Seat above to start one.
              </p>
            </Card>
          )}

          {tablesState === "ready" &&
            tables.map((t) => (
              <button
                key={t.tableId}
                onClick={() => goToTable(t.tableId)}
                className="card-edge group flex items-center justify-between rounded-xl px-4 py-3.5 text-left transition-transform duration-150 hover:-translate-y-[1px] no-tap-highlight"
              >
                <div className="flex items-center gap-3.5">
                  <span className="h-9 w-1.5 rounded-full" style={{ background: TIER_COLOR[tierFor(t.smallBlind)] }} />
                  <div>
                    <p className="num text-base font-semibold tracking-tight">
                      {t.smallBlind} / {t.bigBlind}
                    </p>
                    <p className="text-xs text-ink-faint">Cash game</p>
                  </div>
                </div>
                <span className="num text-sm text-ink-dim">Up to {t.maxSeats} seats</span>
              </button>
            ))}
        </motion.div>

        {/* Right rail */}
        <motion.div className="flex flex-col gap-5" {...rise(0.18)}>
          <Card>
            <SectionLabel>Private</SectionLabel>
            <div className="mt-3 flex flex-col gap-3">
              <Button variant="ghost" onClick={() => setCreating((v) => !v)}>
                {creating ? "Close" : "Create room"}
              </Button>
              <div className="flex gap-2">
                <Input
                  placeholder="6-CHAR CODE"
                  maxLength={6}
                  value={code}
                  onChange={(e) => {
                    setCode(e.target.value.toUpperCase());
                    if (joinError) setJoinError(null);
                  }}
                  className="mono flex-1 uppercase"
                />
                <Button
                  disabled={code.length !== 6 || joining}
                  onClick={() => void joinByCode()}
                >
                  {joining ? "..." : "Join"}
                </Button>
              </div>
              {joinError && (
                <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
                  {joinError}
                </p>
              )}
            </div>
          </Card>

          {creating && (
            <Suspense fallback={<Card>Loading...</Card>}>
              <CreateRoom onClose={() => setCreating(false)} />
            </Suspense>
          )}

          <Card>
            <div className="flex items-center justify-between">
              <SectionLabel>Sit &amp; go</SectionLabel>
              <button
                onClick={() => setCreatingSng((v) => !v)}
                className="text-xs font-semibold uppercase tracking-wider text-ink-dim no-tap-highlight"
              >
                {creatingSng ? "Close" : "Create"}
              </button>
            </div>

            {sngListError && (
              <p className="num mt-2 text-sm" style={{ color: "var(--danger)" }} role="alert">
                {sngListError}
              </p>
            )}

            <div className="mt-3 flex flex-col gap-2">
              {sngs.length === 0 && !sngListError && (
                <p className="text-sm text-ink-faint">No open sit-and-gos right now.</p>
              )}
              {sngs.map((sng) => (
                <SngRow
                  key={sng.sngId}
                  sng={sng}
                  busy={registering === sng.sngId}
                  error={registerError?.id === sng.sngId ? registerError.message : null}
                  onRegister={() => register(sng.sngId)}
                />
              ))}
            </div>
          </Card>

          {creatingSng && (
            <Suspense fallback={<Card>Loading...</Card>}>
              <CreateSng
                onClose={() => setCreatingSng(false)}
                onCreated={() => {
                  setCreatingSng(false);
                  refreshSngs();
                }}
              />
            </Suspense>
          )}

          <FriendsPanel />
        </motion.div>
      </div>

      <div className="flex gap-4 pt-1 text-sm">
        <button className="text-ink-dim underline-offset-4 hover:text-ink hover:underline" onClick={() => nav("/fair")}>
          Provably fair
        </button>
        <button className="text-ink-dim underline-offset-4 hover:text-ink hover:underline" onClick={() => nav("/replay")}>
          Hand replayer
        </button>
        <button className="text-ink-dim underline-offset-4 hover:text-ink hover:underline" onClick={() => nav("/history")}>
          Hand history
        </button>
      </div>
    </Screen>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-xs font-semibold uppercase tracking-[0.18em] text-ink-dim">
      {children}
    </h2>
  );
}

// One sit-and-go listing row: name, buy-in, seats filled/max, and a Register
// CTA. insufficient_funds and other register failures surface inline right
// below the row - never as a modal.
function SngRow({
  sng,
  busy,
  error,
  onRegister,
}: {
  sng: SngView;
  busy: boolean;
  error: string | null;
  onRegister: () => void;
}) {
  const full = sng.registered >= sng.seats;
  return (
    <div className="card-edge flex flex-col gap-1.5 rounded-xl px-3.5 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold">{sng.name}</p>
          <p className="num text-xs text-ink-faint">
            Buy-in {sng.buyIn.toLocaleString("en-US")}, {sng.registered}/{sng.seats} seated
          </p>
        </div>
        <Button
          variant="ghost"
          className="min-h-0 px-3 py-1.5 text-sm"
          disabled={busy || full}
          onClick={onRegister}
        >
          {full ? "Full" : busy ? "Joining..." : "Register"}
        </Button>
      </div>
      {error && (
        <p className="num text-xs font-medium" style={{ color: "var(--danger)" }} role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
