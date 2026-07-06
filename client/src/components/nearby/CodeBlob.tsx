// The copy/paste fallback for serverless WebRTC signaling. QR is the primary
// path now (QrCode / QrScanner), but a text block with one-tap copy and a paste
// target is always available for when the camera is unavailable or a blob will
// not fit a QR. QrCode renders this component beneath the code image, and
// QrScanner reuses BlobInput as its paste fallback, so this stays the single
// source of the copy/paste affordances.

import { useCallback, useState } from "react";

export function CodeBlob({
  value,
  label,
  hint,
}: {
  value: string;
  label: string;
  hint: string;
}) {
  const [copied, setCopied] = useState(false);

  const copy = useCallback(() => {
    void navigator.clipboard?.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    });
  }, [value]);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-sm text-ink-dim">{label}</span>
        <button
          onClick={copy}
          className="no-tap-highlight mono rounded-lg px-3 py-1 text-xs font-medium uppercase tracking-[0.18em] transition-transform duration-150 active:scale-[0.97]"
          style={{ background: "var(--surface-4)", color: copied ? "var(--gold)" : "var(--ink)", boxShadow: "inset 0 0 0 1px var(--line-hi)" }}
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <div
        className="mono max-h-32 overflow-y-auto break-all rounded-xl p-3 text-[11px] leading-relaxed text-ink-dim"
        style={{ background: "var(--surface)", boxShadow: "inset 0 0 0 1px var(--line)" }}
      >
        {value}
      </div>
      <p className="text-xs text-ink-faint">{hint}</p>
    </div>
  );
}

/** A paste target for the peer's blob, with a submit affordance. */
export function BlobInput({
  label,
  hint,
  placeholder,
  onSubmit,
  cta,
  busy,
}: {
  label: string;
  hint: string;
  placeholder: string;
  onSubmit: (value: string) => void;
  cta: string;
  busy?: boolean;
}) {
  const [text, setText] = useState("");
  return (
    <div className="flex flex-col gap-2">
      <span className="text-sm text-ink-dim">{label}</span>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={placeholder}
        rows={4}
        className="mono w-full resize-none break-all rounded-xl border border-line bg-surface p-3 text-[11px] text-ink outline-none transition-shadow placeholder:text-ink-faint focus:border-action-blue focus:shadow-[0_0_0_3px_rgba(76,154,232,0.25)]"
      />
      <p className="text-xs text-ink-faint">{hint}</p>
      <button
        onClick={() => text.trim() && onSubmit(text.trim())}
        disabled={!text.trim() || busy}
        className="no-tap-highlight min-h-[48px] rounded-xl px-5 text-base font-semibold tracking-tight transition-transform duration-150 will-change-transform active:translate-y-[1px] active:scale-[0.99] disabled:pointer-events-none disabled:opacity-50"
        style={{ background: "var(--action-blue)", color: "#fff", boxShadow: "var(--shadow-1)" }}
      >
        {busy ? "Connecting…" : cta}
      </button>
    </div>
  );
}
