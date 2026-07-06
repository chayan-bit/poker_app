// Renders a signaling blob as a scannable QR code (issue #29 QR joining), with
// the compressed text code kept underneath as a copy/paste fallback. The blob is
// compressed first (qrCodec) so it fits QR capacity; if it still will not fit,
// the QR is hidden and only the text code is shown. Uses the installed `qrcode`
// library, rendering to a self-contained data URL.

import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { CodeBlob } from "./CodeBlob";
import { encodeForTransport } from "./qrCodec";

export function QrCode({ value, label, hint }: { value: string; label: string; hint: string }) {
  const [dataUrl, setDataUrl] = useState<string | null>(null);
  const [transport, setTransport] = useState(value);
  const [tooBig, setTooBig] = useState(false);

  useEffect(() => {
    let alive = true;
    void (async () => {
      const payload = await encodeForTransport(value).catch(() => value);
      if (!alive) return;
      setTransport(payload);
      try {
        const url = await QRCode.toDataURL(payload, { errorCorrectionLevel: "L", margin: 2, width: 320 });
        if (!alive) return;
        setDataUrl(url);
        setTooBig(false);
      } catch {
        if (!alive) return;
        setDataUrl(null);
        setTooBig(true);
      }
    })();
    return () => {
      alive = false;
    };
  }, [value]);

  return (
    <div className="flex flex-col gap-3">
      {dataUrl && (
        <div className="flex flex-col items-center gap-2">
          <img
            src={dataUrl}
            alt="Connection QR code"
            width={240}
            height={240}
            className="rounded-xl bg-white p-2"
            style={{ boxShadow: "inset 0 0 0 1px var(--line)" }}
          />
          <p className="text-xs text-ink-faint">Point your friend&apos;s camera at this code.</p>
        </div>
      )}
      {tooBig && (
        <p className="text-xs" style={{ color: "var(--danger, #c0392b)" }}>
          This code is too large to show as a QR - share the text code below instead.
        </p>
      )}
      <CodeBlob value={transport} label={label} hint={hint} />
    </div>
  );
}
