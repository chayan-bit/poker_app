// Camera QR scanning hook (issue #29 QR joining). Prefers the native
// BarcodeDetector API when the WebView exposes it, and falls back to decoding
// getUserMedia video frames with jsQR otherwise. Camera-permission denial and
// unsupported environments are surfaced as status so the caller can degrade to
// the copy/paste fallback rather than wedging. Everything is torn down on stop
// and on unmount so the camera light never lingers.

import { useCallback, useEffect, useRef, useState } from "react";
import jsQR from "jsqr";

export type ScanStatus = "idle" | "scanning" | "denied" | "unsupported";

interface DetectedBarcode {
  rawValue: string;
}
interface BarcodeDetectorLike {
  detect(source: CanvasImageSource): Promise<DetectedBarcode[]>;
}
type BarcodeDetectorCtor = new (opts?: { formats?: string[] }) => BarcodeDetectorLike;

function barcodeDetectorCtor(): BarcodeDetectorCtor | null {
  const ctor = (globalThis as unknown as { BarcodeDetector?: BarcodeDetectorCtor }).BarcodeDetector;
  return typeof ctor === "function" ? ctor : null;
}

/** Decodes a QR from the current video frame via jsQR, or null if none found. */
function scanWithJsqr(video: HTMLVideoElement, canvas: HTMLCanvasElement): string | null {
  const w = video.videoWidth;
  const h = video.videoHeight;
  if (w === 0 || h === 0) return null;
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) return null;
  ctx.drawImage(video, 0, 0, w, h);
  const image = ctx.getImageData(0, 0, w, h);
  const hit = jsQR(image.data, w, h);
  return hit?.data ?? null;
}

export interface QrScanControls {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  status: ScanStatus;
  start: () => Promise<void>;
  stop: () => void;
}

export function useQrScanner(onHit: (raw: string) => void): QrScanControls {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const rafRef = useRef<number | null>(null);
  const detectorRef = useRef<BarcodeDetectorLike | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const onHitRef = useRef(onHit);
  const [status, setStatus] = useState<ScanStatus>("idle");

  // Track the latest callback without resubscribing the scan loop (writing a
  // ref during render is disallowed; do it after commit).
  useEffect(() => {
    onHitRef.current = onHit;
  }, [onHit]);

  const stop = useCallback(() => {
    if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
    rafRef.current = null;
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;
    detectorRef.current = null;
    setStatus("idle");
  }, []);

  const start = useCallback(async () => {
    if (typeof navigator === "undefined" || !navigator.mediaDevices?.getUserMedia) {
      setStatus("unsupported");
      return;
    }

    // Reads one frame, returning the decoded QR text or null.
    async function scanOnce(): Promise<string | null> {
      const video = videoRef.current;
      if (!video || !streamRef.current) return null;
      try {
        if (detectorRef.current) {
          const codes = await detectorRef.current.detect(video);
          return codes[0]?.rawValue ?? null;
        }
        if (!canvasRef.current) canvasRef.current = document.createElement("canvas");
        return scanWithJsqr(video, canvasRef.current);
      } catch {
        return null;
      }
    }

    // Hoisted declaration so the recursive rAF reschedule can reference it
    // without a use-before-declare on a self-referential arrow.
    function frame(): void {
      void scanOnce().then((raw) => {
        if (!streamRef.current) return; // stopped while detecting
        if (raw) {
          onHitRef.current(raw);
          return;
        }
        rafRef.current = requestAnimationFrame(frame);
      });
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" } });
      const video = videoRef.current;
      if (!video) {
        stream.getTracks().forEach((t) => t.stop());
        return;
      }
      streamRef.current = stream;
      video.srcObject = stream;
      video.setAttribute("playsinline", "true");
      await video.play();
      const Ctor = barcodeDetectorCtor();
      detectorRef.current = Ctor ? new Ctor({ formats: ["qr_code"] }) : null;
      setStatus("scanning");
      rafRef.current = requestAnimationFrame(frame);
    } catch (err) {
      const name = (err as { name?: string })?.name;
      setStatus(name === "NotAllowedError" || name === "SecurityError" ? "denied" : "unsupported");
    }
  }, []);

  useEffect(() => stop, [stop]);

  return { videoRef, status, start, stop };
}
