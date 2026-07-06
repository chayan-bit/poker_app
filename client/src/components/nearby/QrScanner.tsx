// Scans a peer's QR code with the camera (issue #29 QR joining) and hands the
// decoded rtc blob back. Falls back to a copy/paste text field whenever the
// camera is unavailable or permission is denied, so joining always has a path.
// A scanned value is decompressed by qrCodec before being surfaced.

import { useCallback, useState } from "react";
import { BlobInput } from "./CodeBlob";
import { decodeFromTransport } from "./qrCodec";
import { useQrScanner, type ScanStatus } from "./qrScan";

function CameraIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
      <circle cx="12" cy="13" r="4" />
    </svg>
  );
}

function statusNote(status: ScanStatus): string | null {
  if (status === "denied") return "Camera access was blocked. Paste the code instead, or allow the camera and retry.";
  if (status === "unsupported") return "This device cannot scan here. Paste the code instead.";
  return null;
}

export function QrScanner({
  label,
  hint,
  cta,
  busy,
  onResult,
}: {
  label: string;
  hint: string;
  cta: string;
  busy?: boolean;
  onResult: (blob: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);

  const onHit = useCallback(
    (raw: string) => {
      void (async () => {
        try {
          onResult(await decodeFromTransport(raw));
        } catch {
          setError("That code could not be read. Try again, or paste it below.");
        }
      })();
    },
    [onResult],
  );

  const { videoRef, status, start, stop } = useQrScanner(onHit);
  const note = statusNote(status);

  const onPaste = useCallback(
    (text: string) => {
      void (async () => {
        try {
          onResult(await decodeFromTransport(text));
        } catch {
          setError("That code is not a valid invite. Check you copied all of it.");
        }
      })();
    },
    [onResult],
  );

  return (
    <div className="flex flex-col gap-3">
      <span className="text-sm text-ink-dim">{label}</span>

      {status === "scanning" ? (
        <div className="flex flex-col gap-2">
          <video
            ref={videoRef}
            muted
            playsInline
            className="aspect-square w-full rounded-xl object-cover"
            style={{ background: "var(--surface)", boxShadow: "inset 0 0 0 1px var(--line)" }}
          />
          <button
            onClick={stop}
            className="no-tap-highlight min-h-[44px] rounded-xl text-sm font-medium text-ink-dim transition-transform duration-150 active:scale-[0.98]"
            style={{ background: "var(--surface-4)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
          >
            Stop camera
          </button>
        </div>
      ) : (
        <button
          onClick={() => {
            setError(null);
            void start();
          }}
          className="no-tap-highlight flex min-h-[48px] items-center justify-center gap-2 rounded-xl px-5 text-base font-semibold transition-transform duration-150 active:scale-[0.99]"
          style={{ background: "var(--action-blue)", color: "#fff", boxShadow: "var(--shadow-1)" }}
        >
          <CameraIcon />
          Scan with camera
        </button>
      )}

      {note && <p className="text-xs" style={{ color: "var(--ink-dim)" }}>{note}</p>}
      {error && <p className="text-xs" style={{ color: "var(--danger, #c0392b)" }}>{error}</p>}

      <BlobInput
        label="Or paste the code"
        placeholder="Paste the code here"
        hint={hint}
        cta={cta}
        busy={busy}
        onSubmit={onPaste}
      />
    </div>
  );
}
