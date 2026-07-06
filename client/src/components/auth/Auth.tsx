// Auth screens: email + OAuth login, plus "continue as guest". Guest state
// persists via a device token and can later upgrade to a full account without
// losing chips/history. Client-side flows only - these call the server's auth
// endpoints (stubbed here; wire to the real endpoints when integrating).

import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card, Button, Field, Input } from "@/components/ui/kit";

const DEVICE_TOKEN_KEY = "poker:deviceToken";

function ensureDeviceToken(): string {
  let t = localStorage.getItem(DEVICE_TOKEN_KEY);
  if (!t) {
    t = crypto.randomUUID();
    localStorage.setItem(DEVICE_TOKEN_KEY, t);
  }
  return t;
}

export default function Auth() {
  const nav = useNavigate();
  const [email, setEmail] = useState("");

  return (
    <Screen title="Sign in" back={<BackButton />}>
      <Card>
        <div className="flex flex-col gap-4">
          <Field label="Email">
            <Input
              type="email"
              placeholder="you@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </Field>
          <Button disabled={!email.includes("@")} onClick={() => nav("/lobby")}>
            Continue with email
          </Button>
          <div className="flex items-center gap-3 text-xs text-ink-faint">
            <span className="h-px flex-1 bg-line" /> or{" "}
            <span className="h-px flex-1 bg-line" />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <Button variant="ghost" onClick={() => nav("/lobby")}>
              Google
            </Button>
            <Button variant="ghost" onClick={() => nav("/lobby")}>
              Apple
            </Button>
          </div>
        </div>
      </Card>

      <Card>
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="font-medium">Continue as guest</p>
            <p className="text-sm text-ink-dim">
              Your chips and history are saved to this device. Upgrade anytime.
            </p>
          </div>
          <Button
            variant="gold"
            onClick={() => {
              ensureDeviceToken();
              nav("/lobby");
            }}
          >
            Guest
          </Button>
        </div>
      </Card>
    </Screen>
  );
}

function BackButton() {
  const nav = useNavigate();
  return (
    <button
      onClick={() => nav("/")}
      className="grid h-9 w-9 place-items-center rounded-lg border border-line text-ink-dim"
      aria-label="Back"
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M15 5l-7 7 7 7" /></svg>
    </button>
  );
}
