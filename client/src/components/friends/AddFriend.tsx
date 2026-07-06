// Add-friend entry: a single input that sends POST /api/friends/request.
// The server only accepts a playerId (see server/internal/social/handlers.go
// decodePlayerID) - there is no name-search endpoint - so this field takes
// either a player id or a display name and sends it verbatim as playerId;
// if the backing store keys friend requests by id only, a name that isn't
// also the id will surface the server's own "no_such_request"/not-found style
// error inline, quietly, same as any other request failure here.

import { useState } from "react";
import { Button, Input } from "@/components/ui/kit";
import { ApiError } from "@/net/api";
import { requestFriend } from "@/net/social";

export function AddFriend({ onSent }: { onSent: () => void }) {
  const [value, setValue] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  async function submit() {
    const target = value.trim();
    if (!target || pending) return;
    setPending(true);
    setError(null);
    try {
      await requestFriend(target);
      setSent(true);
      setValue("");
      onSent();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not send that request");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex gap-2">
        <Input
          placeholder="Player id or name"
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            setSent(false);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") void submit();
          }}
          className="flex-1"
        />
        <Button variant="ghost" disabled={!value.trim() || pending} onClick={() => void submit()}>
          {pending ? "Sending…" : "Add"}
        </Button>
      </div>
      {error && <p className="text-xs" style={{ color: "var(--danger)" }}>{error}</p>}
      {sent && !error && <p className="text-xs text-ink-faint">Request sent.</p>}
    </div>
  );
}
