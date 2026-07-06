// Maps the internal errors thrown along the nearby setup path (WASM load, RTC
// signaling, malformed blobs) to a short, user-facing sentence with a retry
// framing. Keeps the copy in one place so host and join flows read the same.

/** Turns any thrown value into a clear, non-technical message for the UI. */
export function errorMessage(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? "");
  const msg = raw.toLowerCase();

  if (msg.includes("webassembly is not available")) {
    return "This device can't run the offline table (WebAssembly is unavailable). Try a different browser.";
  }
  if (msg.includes("localcore") || msg.includes("tablecore") || msg.includes("wasm")) {
    return "The offline table failed to load. Check your connection and try again.";
  }
  if (msg.includes("malformed") || msg.includes("missing required fields") || msg.includes("not a valid")) {
    return "That code is malformed or incomplete. Ask for a fresh one and try again.";
  }
  if (msg.includes("missing the table settings")) {
    return "This invite is missing the table settings. Ask your host for a fresh invite.";
  }
  if (msg.includes("timed out") || msg.includes("timeout")) {
    return "Connection timed out. Make sure you are on the same network, then try again.";
  }
  if (msg.includes("ice") || msg.includes("connection")) {
    return "Could not connect. Make sure you are on the same Wi-Fi and try again.";
  }
  return raw || "Something went wrong. Please try again.";
}
