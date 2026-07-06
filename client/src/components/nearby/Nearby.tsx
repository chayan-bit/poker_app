// Route container for "Play with friends offline". Owns the offline-mode guard
// flag for its whole lifetime and the single NearbySession instance, and swaps
// between the chooser, host setup, join flow, live table, and summary by phase.

import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { motion } from "framer-motion";
import { Card, Screen } from "@/components/ui/kit";
import { setOfflineMode } from "@/net/mode";
import { useNearby } from "./nearbyStore";
import { NearbySession } from "./session";
import HostSetup from "./HostSetup";
import JoinFlow from "./JoinFlow";
import NearbyTable from "./NearbyTable";
import SessionSummary from "./SessionSummary";

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      aria-label="Back"
      className="no-tap-highlight grid h-10 w-10 place-items-center rounded-xl text-ink-dim transition-transform duration-150 active:scale-[0.96]"
      style={{ background: "var(--surface-4)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <path d="M15 18l-6-6 6-6" />
      </svg>
    </button>
  );
}

export default function Nearby() {
  const nav = useNavigate();
  const phase = useNearby((s) => s.phase);
  const setPhase = useNearby((s) => s.setPhase);
  const reset = useNearby((s) => s.reset);
  const sessionRef = useRef<NearbySession | null>(null);

  useEffect(() => {
    setOfflineMode(true);
    return () => {
      setOfflineMode(false);
      sessionRef.current?.dispose();
      sessionRef.current = null;
      reset();
    };
  }, [reset]);

  const onReady = (s: NearbySession) => {
    sessionRef.current = s;
  };

  if (phase === "table") return <NearbyTable session={sessionRef.current} />;

  if (phase === "summary") {
    return (
      <Screen>
        <SessionSummary
          onDone={() => {
            sessionRef.current?.dispose();
            sessionRef.current = null;
            reset();
            nav("/");
          }}
        />
      </Screen>
    );
  }

  const back = () => {
    if (phase === "setup") {
      nav("/");
    } else {
      sessionRef.current?.dispose();
      sessionRef.current = null;
      setPhase("setup");
    }
  };

  return (
    <Screen title="Play with friends offline" back={<BackButton onClick={back} />}>
      {phase === "setup" && <Chooser />}
      {phase === "host" && <HostSetup onReady={onReady} />}
      {phase === "join" && <JoinFlow onReady={onReady} />}
    </Screen>
  );
}

function Chooser() {
  const setPhase = useNearby((s) => s.setPhase);
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4 }}
      className="flex flex-col gap-4"
    >
      <p className="text-ink-dim">
        Play over the local network with no server and no sign-in. Start a table
        and invite friends, or join one you have been invited to.
      </p>
      <Card>
        <button className="no-tap-highlight flex w-full flex-col gap-1 text-left" onClick={() => setPhase("host")}>
          <span className="display text-xl">Host a table</span>
          <span className="text-sm text-ink-dim">Set the stakes and invite friends by code.</span>
        </button>
      </Card>
      <Card>
        <button className="no-tap-highlight flex w-full flex-col gap-1 text-left" onClick={() => setPhase("join")}>
          <span className="display text-xl">Join a table</span>
          <span className="text-sm text-ink-dim">Paste an invite code to take a seat.</span>
        </button>
      </Card>
      <p className="text-xs text-ink-faint">
        Chips are for the table only. They never sync to your online balance.
      </p>
    </motion.div>
  );
}
