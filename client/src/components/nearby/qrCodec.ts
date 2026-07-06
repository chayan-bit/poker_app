// Compression codec for the WebRTC signaling blobs (issue #29 QR joining).
//
// The rtc.ts offer/answer blob is base64(JSON({peerId, sdp, payload})). The SDP
// dominates its size (1-2 KB) and is highly repetitive, so it compresses well.
// We recover the underlying JSON, deflate it with the platform CompressionStream
// (present in modern WebViews incl. Capacitor iOS 16.4+/Android), and re-encode
// the compressed bytes as base64 - typically a 55-70% size reduction, small
// enough to render as a scannable QR code. Where CompressionStream is missing we
// fall back to the raw blob so copy/paste always works.
//
// The transport string is self-describing via a short magic prefix, so a decoder
// accepts a compressed QR payload, an uncompressed one, OR a legacy raw blob
// pasted directly - the paste fallback never breaks.

const TAG_DEFLATE = "Q1:";
const TAG_RAW = "Q0:";
const DEFLATE_FORMAT = "deflate-raw";

function hasCompression(): boolean {
  return typeof CompressionStream !== "undefined" && typeof DecompressionStream !== "undefined";
}

function bytesToBase64(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function base64ToBytes(b64: string): Uint8Array {
  const s = atob(b64);
  const out = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i += 1) out[i] = s.charCodeAt(i);
  return out;
}

/** Decodes a base64 string to its text, or null if it is not valid base64. */
function safeAtob(b64: string): string | null {
  try {
    return atob(b64);
  } catch {
    return null;
  }
}

async function deflate(text: string): Promise<Uint8Array> {
  const cs = new CompressionStream(DEFLATE_FORMAT);
  const stream = new Blob([new TextEncoder().encode(text)]).stream().pipeThrough(cs);
  const buf = await new Response(stream).arrayBuffer();
  return new Uint8Array(buf);
}

async function inflate(bytes: Uint8Array): Promise<string> {
  const ds = new DecompressionStream(DEFLATE_FORMAT);
  const stream = new Blob([bytes as unknown as BlobPart]).stream().pipeThrough(ds);
  const buf = await new Response(stream).arrayBuffer();
  return new TextDecoder().decode(buf);
}

/**
 * Encodes an rtc blob into the smallest transport string that still round-trips.
 * Compresses the underlying JSON when possible; otherwise passes the blob
 * through so copy/paste still works on browsers without CompressionStream.
 */
export async function encodeForTransport(blob: string): Promise<string> {
  const json = safeAtob(blob);
  if (json !== null && hasCompression()) {
    try {
      const compressed = await deflate(json);
      return TAG_DEFLATE + bytesToBase64(compressed);
    } catch {
      // Fall through to raw on any codec error.
    }
  }
  return TAG_RAW + blob;
}

/**
 * Decodes a transport string back to the rtc blob rtc.ts expects. Accepts a
 * compressed payload, an uncompressed one, or a legacy raw blob (no prefix), so
 * pasting either a scanned QR payload or an older client's blob both work.
 */
export async function decodeFromTransport(text: string): Promise<string> {
  const s = text.trim();
  if (s.startsWith(TAG_DEFLATE)) {
    const json = await inflate(base64ToBytes(s.slice(TAG_DEFLATE.length)));
    return btoa(json);
  }
  if (s.startsWith(TAG_RAW)) {
    return s.slice(TAG_RAW.length);
  }
  return s;
}
