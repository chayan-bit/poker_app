// Landing / instant join. A private game is just a URL (/t/:joinCode). Opening
// it seats you as a guest after a single name prompt - nothing else. Also the
// home for logged-in users. The hero is asymmetric: a confident type column and
// a dramatic, gently-floating royal-flush fan lit over the felt.

import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { motion } from "framer-motion";
import { Screen, Card as UICard, Button, Field, Input, Icon, SpadeMark } from "@/components/ui/kit";
import { Card } from "@/components/table/Card";
import { ChipStack } from "@/components/table/Chips";

const EASE = [0.22, 1, 0.36, 1] as const;
const rise = (delay: number) => ({
  initial: { opacity: 0, y: 14 },
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0.55, ease: EASE, delay },
});

function HeroFan() {
  // Five cards fanned around a pivot; the whole fan floats as one unit.
  const fan = [
    { c: "Ah", rot: -22, x: -128, y: 22 },
    { c: "Kh", rot: -11, x: -66, y: 4 },
    { c: "Qh", rot: 0, x: 0, y: -4 },
    { c: "Jh", rot: 11, x: 66, y: 4 },
    { c: "Th", rot: 22, x: 128, y: 22 },
  ];
  return (
    <div className="relative grid h-64 w-full place-items-center">
      {/* felt spotlight */}
      <div
        className="pointer-events-none absolute h-72 w-72 rounded-full blur-[60px]"
        style={{ background: "radial-gradient(circle, color-mix(in oklab, var(--felt-hi), transparent 40%), transparent 70%)" }}
      />
      <div className="hero-float relative h-full w-full">
        {fan.map((f, i) => (
          <motion.div
            key={f.c}
            className="absolute left-1/2 top-1/2"
            style={{ zIndex: i }}
            initial={{ opacity: 0, y: 40, rotate: 0 }}
            animate={{ opacity: 1, y: f.y, rotate: f.rot, x: f.x }}
            transition={{ duration: 0.6, ease: EASE, delay: 0.15 + i * 0.07 }}
          >
            <div style={{ transform: "translate(-50%, -50%)" }}>
              <Card card={f.c} size="lg" />
            </div>
          </motion.div>
        ))}
      </div>
      <motion.div className="absolute bottom-1 left-1/2 -translate-x-1/2" {...rise(0.6)}>
        <ChipStack amount={1240} size={30} />
      </motion.div>
    </div>
  );
}

export default function Landing() {
  const { joinCode } = useParams();
  const nav = useNavigate();
  const [name, setName] = useState("");

  if (joinCode) {
    return (
      <Screen>
        <div className="flex min-h-[80vh] flex-col justify-center gap-7">
          <motion.div className="text-center" {...rise(0)}>
            <p className="mono text-xs font-medium uppercase tracking-[0.3em] text-ink-faint">
              Table · {joinCode}
            </p>
            <h1 className="display mt-3 text-4xl">
              You're invited <span className="display-accent">to play</span>
            </h1>
            <p className="mt-2 text-ink-dim">One name and you're seated. No signup.</p>
          </motion.div>
          <motion.div {...rise(0.1)}>
            <UICard>
              <div className="flex flex-col gap-4">
                <Field label="Pick a name">
                  <Input autoFocus placeholder="e.g. Ace" value={name} onChange={(e) => setName(e.target.value)} />
                </Field>
                <Button
                  variant="gold"
                  disabled={!name.trim()}
                  onClick={() => nav(`/table?join=${joinCode}&guest=${encodeURIComponent(name)}`)}
                >
                  Take my seat
                </Button>
                <button className="text-sm text-ink-dim transition-colors hover:text-ink" onClick={() => nav("/auth")}>
                  or sign in first
                </button>
              </div>
            </UICard>
          </motion.div>
        </div>
      </Screen>
    );
  }

  return (
    <Screen wide>
      <div className="flex min-h-[88vh] flex-col justify-center gap-14 py-6 md:grid md:grid-cols-[1.05fr_0.95fr] md:items-center md:gap-8">
        {/* Type column */}
        <div className="flex flex-col items-start gap-6">
          <motion.span
            className="num inline-flex items-center gap-2 rounded-full px-3 py-1 text-[11px] font-medium uppercase tracking-[0.28em] text-ink-dim"
            style={{ background: "var(--surface-3)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
            {...rise(0)}
          >
            <span style={{ color: "var(--gold)" }}>
              <SpadeMark size={14} />
            </span>
            Private card room
          </motion.span>

          <motion.h1
            className="display text-[clamp(2.7rem,7vw,4.6rem)] leading-[1.02]"
            {...rise(0.08)}
          >
            Poker that
            <br />
            feels <span className="display-accent">instant</span>.
          </motion.h1>

          <motion.p className="max-w-md text-lg leading-relaxed text-ink-dim" {...rise(0.16)}>
            A calm room where every tap lands before the server even answers.
            Deal a private table from a single link, or sit down in seconds.
          </motion.p>

          <motion.div className="flex w-full max-w-md flex-col gap-3 sm:flex-row" {...rise(0.24)}>
            <Button variant="gold" className="flex-1" onClick={() => nav("/table")}>
              Play a demo hand
            </Button>
            <Button variant="ghost" className="flex-1" onClick={() => nav("/lobby")}>
              Browse tables
            </Button>
          </motion.div>

          <motion.div className="mt-2 flex flex-wrap gap-x-6 gap-y-3 text-sm text-ink-dim" {...rise(0.32)}>
            <Feature icon="bolt" label="Sub-100ms actions" />
            <Feature icon="shield" label="Provably fair" />
            <Feature icon="devices" label="iOS · Android · Web" />
          </motion.div>

          <motion.button
            className="text-sm text-ink-faint underline-offset-4 transition-colors hover:text-ink-dim hover:underline"
            onClick={() => nav("/auth")}
            {...rise(0.4)}
          >
            Sign in or continue as guest
          </motion.button>
        </div>

        {/* Hero visual */}
        <div className="order-first md:order-none">
          <HeroFan />
        </div>
      </div>
    </Screen>
  );
}

function Feature({ icon, label }: { icon: "bolt" | "shield" | "devices"; label: string }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span style={{ color: "var(--gold)" }}>
        <Icon name={icon} size={18} />
      </span>
      {label}
    </span>
  );
}
