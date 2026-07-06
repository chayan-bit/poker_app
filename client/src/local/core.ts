// Typed TypeScript wrapper around the Go table core compiled to WebAssembly
// (see server/cmd/tablewasm and server/internal/localcore). It loads Go's
// wasm_exec.js runtime, instantiates tablecore.wasm, and exposes a small typed
// API so the client can run the authoritative game logic locally for offline LAN
// play (issue #27). No framework imports; safe to use from any context.
//
// The JS boundary is all-strings/JSON by design: every call passes and returns
// JSON strings, and all randomness (the seed) and all time (nowMs) are supplied
// by the host, so an offline hand is byte-identical to an online one.

/** Host-chosen ruleset for a local table. Mirrors localcore.Config. */
export interface LocalConfig {
  id: string;
  maxSeats?: number;
  smallBlind: number;
  bigBlind: number;
  hostPlayerId?: string;
  /** Deal automatically once two seats are ready (default for public tables). */
  autoStart?: boolean;
  /** Private room: the host must send start_hand once before hands auto-continue. */
  private?: boolean;
  turnTimeoutMs?: number;
  disconnectGraceMs?: number;
}

/** A wire message. Mirrors protocol.Envelope. */
export interface Envelope {
  v: number;
  type: string;
  seq?: number;
  data?: unknown;
}

/**
 * Per-recipient event envelopes. The "*" key carries table-wide (broadcast)
 * events; any other key is a player ID receiving a privacy send (hole cards,
 * targeted snapshot, error). Merge the two streams by `seq`, exactly as online.
 */
export type EventMap = Record<string, Envelope[]>;

/** Recipient key under which broadcast events are collected. */
export const BROADCAST = "*";

// ---- Go WASM runtime typings (wasm_exec.js provides globalThis.Go) ----

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

interface GoConstructor {
  new (): GoRuntime;
}

/** The handle object the WASM module returns from newTable. */
interface TableHandle {
  submit(playerId: string, envelopeJson: string): string;
  tick(nowMs: number): string;
  stateHash(): string;
  voidHand(): string;
  setSeed(seedHex: string): void;
}

interface TableCore {
  newTable(configJson: string, seedHex: string): TableHandle | { error: string };
}

declare global {
  // eslint-disable-next-line no-var
  var Go: GoConstructor | undefined;
  // eslint-disable-next-line no-var
  var tablecore: TableCore | undefined;
}

let runtimeReady: Promise<void> | null = null;

/**
 * Loads Go's wasm_exec.js runtime and instantiates the table core exactly once.
 * Subsequent calls return the same promise. Paths default to the Vite public
 * root; override for a different asset layout.
 */
export function initLocalCore(
  wasmUrl = "/tablecore.wasm",
  execUrl = "/wasm_exec.js",
): Promise<void> {
  if (runtimeReady) return runtimeReady;
  runtimeReady = (async () => {
    if (typeof WebAssembly === "undefined") {
      throw new Error("localcore: WebAssembly is not available in this environment");
    }
    if (typeof globalThis.Go === "undefined") {
      await loadScript(execUrl);
    }
    if (typeof globalThis.Go === "undefined") {
      throw new Error("localcore: wasm_exec.js did not define globalThis.Go");
    }
    const go = new globalThis.Go!();
    const instance = await instantiate(wasmUrl, go.importObject);
    // Do NOT await: the Go program blocks on select{} for the tab's lifetime so
    // its exports stay callable. run() only settles when the module exits.
    void go.run(instance);
    // The module installs globalThis.tablecore synchronously during run(); it is
    // available on the next microtask.
    await Promise.resolve();
    if (typeof globalThis.tablecore === "undefined") {
      throw new Error("localcore: WASM module did not install globalThis.tablecore");
    }
  })();
  return runtimeReady;
}

/** Loads a classic (non-module) script and resolves when it has executed. */
function loadScript(src: string): Promise<void> {
  if (typeof document === "undefined") {
    return Promise.reject(new Error("localcore: no document to load wasm_exec.js; call from a browser"));
  }
  return new Promise((resolve, reject) => {
    const el = document.createElement("script");
    el.src = src;
    el.async = false;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error(`localcore: failed to load ${src}`));
    document.head.appendChild(el);
  });
}

/** Instantiates the wasm module, preferring streaming with a fetch fallback. */
async function instantiate(
  wasmUrl: string,
  importObject: WebAssembly.Imports,
): Promise<WebAssembly.Instance> {
  const resp = fetch(wasmUrl);
  if (WebAssembly.instantiateStreaming) {
    try {
      const result = await WebAssembly.instantiateStreaming(resp, importObject);
      return result.instance;
    } catch {
      // Fall through: some servers send the wrong MIME type for .wasm.
    }
  }
  const bytes = await (await fetch(wasmUrl)).arrayBuffer();
  const result = await WebAssembly.instantiate(bytes, importObject);
  return result.instance;
}

/**
 * A local authoritative poker table running inside WASM. Construct after
 * initLocalCore() has resolved. Every method returns the same per-recipient
 * event shape the WebSocket path produces.
 */
export class LocalCore {
  private readonly handle: TableHandle;

  constructor(config: LocalConfig, seedHex: string) {
    if (typeof globalThis.tablecore === "undefined") {
      throw new Error("localcore: call and await initLocalCore() before constructing LocalCore");
    }
    const handle = globalThis.tablecore.newTable(JSON.stringify(config), seedHex);
    if (isError(handle)) {
      throw new Error(`localcore: newTable failed: ${handle.error}`);
    }
    this.handle = handle;
  }

  /** Applies one client command and returns the resulting per-recipient events. */
  submit(playerId: string, envelope: Envelope | string): EventMap {
    const json = typeof envelope === "string" ? envelope : JSON.stringify(envelope);
    return parseEvents(this.handle.submit(playerId, json));
  }

  /**
   * Advances the table's notion of time to nowMs (epoch milliseconds) and
   * returns any events from expired turn/disconnect deadlines. Time is purely an
   * input; nowMs must never regress.
   */
  tick(nowMs: number): EventMap {
    return parseEvents(this.handle.tick(nowMs));
  }

  /** SHA-256 hex of the canonical table state, for peer snapshot verification. */
  stateHash(): string {
    return this.handle.stateHash();
  }

  /** Aborts the in-flight hand, returning committed chips to their stacks. */
  voidHand(): EventMap {
    return parseEvents(this.handle.voidHand());
  }

  /** Sets the 32-byte hex seed to commit and shuffle for the next hand. */
  setSeed(seedHex: string): void {
    this.handle.setSeed(seedHex);
  }
}

/** Draws a fresh 32-byte seed as a 64-char hex string from the platform CSPRNG. */
export function newSeedHex(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  let hex = "";
  for (const b of bytes) hex += b.toString(16).padStart(2, "0");
  return hex;
}

function isError(v: TableHandle | { error: string }): v is { error: string } {
  return typeof (v as { error?: unknown }).error === "string";
}

function parseEvents(json: string): EventMap {
  if (!json) return {};
  const parsed = JSON.parse(json) as EventMap | null;
  return parsed ?? {};
}
