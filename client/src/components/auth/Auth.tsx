// Auth screen. The server exposes exactly two identity endpoints: POST
// /api/auth/guest (issues a signed guest token) and POST /api/auth/upgrade
// (attaches an email to the current guest, preserving chips/history). There is
// no email/password or OAuth login on the wire, so the real flow is: establish
// a guest session on entry, then optionally upgrade it to a named account.
//
// Every failure surfaces inline (ApiError code + message) - never as an alert
// or modal.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Screen, Card, Button, Field, Input } from "@/components/ui/kit";
import { guestLogin, upgrade, getStoredToken, ApiError } from "@/net/api";

type SessionState = "initializing" | "ready" | "error";

function isValidEmail(value: string): boolean {
  // Boundary validation only; the server is authoritative on uniqueness.
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}

export default function Auth() {
  const nav = useNavigate();
  const [email, setEmail] = useState("");
  const [session, setSession] = useState<SessionState>(
    getStoredToken() ? "ready" : "initializing",
  );
  const [sessionError, setSessionError] = useState<string | null>(null);
  const [upgrading, setUpgrading] = useState(false);
  const [upgradeError, setUpgradeError] = useState<string | null>(null);

  // Ensure a real guest session exists before the user can reach the lobby.
  // If a token is already stored we keep that identity (chips/history) rather
  // than minting a new guest.
  const establishGuestSession = async (): Promise<boolean> => {
    if (getStoredToken()) {
      setSession("ready");
      return true;
    }
    setSession("initializing");
    setSessionError(null);
    try {
      await guestLogin();
      setSession("ready");
      return true;
    } catch (err) {
      setSession("error");
      setSessionError(
        err instanceof ApiError
          ? `${err.message} (${err.code})`
          : "Could not reach the server. Check your connection and retry.",
      );
      return false;
    }
  };

  useEffect(() => {
    if (!getStoredToken()) void establishGuestSession();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const continueAsGuest = async () => {
    const ok = getStoredToken() ? true : await establishGuestSession();
    if (ok) nav("/lobby");
  };

  const upgradeWithEmail = async () => {
    setUpgradeError(null);
    if (!isValidEmail(email)) {
      setUpgradeError("Enter a valid email address.");
      return;
    }
    // Upgrade requires an authenticated guest; make sure one exists first.
    const hasSession = getStoredToken() ? true : await establishGuestSession();
    if (!hasSession) return;
    setUpgrading(true);
    try {
      await upgrade(email.trim());
      nav("/lobby");
    } catch (err) {
      setUpgradeError(
        err instanceof ApiError
          ? `${err.message} (${err.code})`
          : "Could not upgrade your account. Please try again.",
      );
    } finally {
      setUpgrading(false);
    }
  };

  const isSessionReady = session === "ready";

  return (
    <Screen title="Sign in" back={<BackButton />}>
      {session === "initializing" && (
        <Card>
          <p className="text-sm text-ink-dim" role="status">
            Setting up your session...
          </p>
        </Card>
      )}

      {session === "error" && (
        <Card>
          <div className="flex flex-col gap-3">
            <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
              {sessionError}
            </p>
            <Button variant="ghost" onClick={() => void establishGuestSession()}>
              Retry
            </Button>
          </div>
        </Card>
      )}

      <Card>
        <div className="flex flex-col gap-4">
          <div>
            <p className="font-medium">Save your progress</p>
            <p className="text-sm text-ink-dim">
              Add an email to secure your chips and history across devices.
            </p>
          </div>
          <Field label="Email">
            <Input
              type="email"
              placeholder="you@example.com"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                if (upgradeError) setUpgradeError(null);
              }}
              disabled={upgrading}
            />
          </Field>
          {upgradeError && (
            <p className="num text-sm font-medium" style={{ color: "var(--danger)" }} role="alert">
              {upgradeError}
            </p>
          )}
          <Button
            variant="gold"
            disabled={!isSessionReady || upgrading || !isValidEmail(email)}
            onClick={() => void upgradeWithEmail()}
          >
            {upgrading ? "Saving..." : "Continue with email"}
          </Button>
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
            variant="ghost"
            disabled={session === "initializing"}
            onClick={() => void continueAsGuest()}
          >
            {session === "initializing" ? "..." : "Guest"}
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
