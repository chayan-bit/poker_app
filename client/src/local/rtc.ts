// WebRTC data-channel transport with serverless QR signaling (issue #28).
//
// A browser guest joins the mesh with no signaling server: the offerer produces
// a compact blob (SDP + gathered ICE candidates + the host's table config), the
// answerer consumes it and produces an answer blob, and the two are exchanged
// out of band (a QR code or a pasted string each way; QR rendering lives in the
// nearby components). On a LAN, host candidates alone connect, so no STUN/TURN
// is configured by default; a STUN server can be supplied via env (VITE_STUN_URL)
// for non-LAN use without touching this code.
//
// This module manufactures the blob strings and the Connection, and it owns
// failure handling: every promise it hands back either resolves or rejects (on
// ICE/negotiation failure or timeout) - none can hang forever - and it tears the
// RTCPeerConnection down on failure so the caller never leaks a wedged socket.

import type { Connection } from "./transport.ts";

/** Terminal-ish connection state surfaced so the UI can show progress/failure. */
export type RtcState = "connecting" | "connected" | "disconnected" | "failed" | "closed";

/** Tunables for a signaling session. All optional with LAN-sane defaults. */
export interface RtcOptions {
  /** ICE servers. Empty (default) means pure-LAN host candidates only. */
  iceServers?: RTCIceServer[];
  /** Cap on ICE gathering before we proceed with whatever candidates we have. */
  gatherTimeoutMs?: number;
  /** Cap on waiting for the data channel to open before rejecting. */
  connectTimeoutMs?: number;
  /** Notified on every connection-state transition (connecting/failed/...). */
  onState?: (state: RtcState) => void;
}

const CHANNEL_LABEL = "feltpoker-mesh";
const DEFAULT_GATHER_TIMEOUT_MS = 4000;
const DEFAULT_CONNECT_TIMEOUT_MS = 20_000;
const FATAL_STATES = new Set(["failed", "closed"]);

/** Reads an optional STUN url from the Vite env; absent means pure LAN. */
function defaultIceServers(): RTCIceServer[] {
  const env = (import.meta as unknown as { env?: Record<string, string | undefined> }).env;
  const url = env?.VITE_STUN_URL;
  return url ? [{ urls: url }] : [];
}

function rtcConfig(opts: RtcOptions): RTCConfiguration {
  return { iceServers: opts.iceServers ?? defaultIceServers() };
}

/** The wire blob exchanged out of band; `payload` carries the host's config. */
interface SignalBlob {
  peerId: string;
  sdp: RTCSessionDescriptionInit;
  payload?: Record<string, unknown>;
}

function encodeBlob(blob: SignalBlob): string {
  return btoa(JSON.stringify(blob));
}

/** Decodes an out-of-band blob; throws a clear error on malformed input. */
function decodeBlob(text: string): SignalBlob {
  let blob: SignalBlob;
  try {
    blob = JSON.parse(atob(text.trim())) as SignalBlob;
  } catch {
    throw new Error("rtc: invite/answer code is malformed or incomplete");
  }
  if (!blob || typeof blob.peerId !== "string" || !blob.sdp || typeof blob.sdp.type !== "string") {
    throw new Error("rtc: invite/answer code is missing required fields");
  }
  return blob;
}

/** Resolves once ICE gathering completes, or after a timeout with what we have. */
function whenGathered(pc: RTCPeerConnection, timeoutMs: number): Promise<void> {
  if (pc.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    const cleanup = (): void => {
      clearTimeout(timer);
      pc.removeEventListener("icegatheringstatechange", check);
    };
    const check = (): void => {
      if (pc.iceGatheringState === "complete") {
        cleanup();
        resolve();
      }
    };
    // Non-fatal: on a LAN, host candidates arrive fast; proceed with whatever
    // gathered rather than blocking the whole handshake on trickle completion.
    const timer = setTimeout(() => {
      cleanup();
      resolve();
    }, timeoutMs);
    pc.addEventListener("icegatheringstatechange", check);
  });
}

/** Wraps an RTCDataChannel as a mesh Connection, surfacing pc state changes. */
class RtcConnection implements Connection {
  readonly peerId: string;
  private msgCb: ((d: string) => void) | null = null;
  private closeCb: (() => void) | null = null;

  constructor(
    peerId: string,
    private readonly channel: RTCDataChannel,
    private readonly pc: RTCPeerConnection,
    private readonly onState?: (state: RtcState) => void,
  ) {
    this.peerId = peerId;
    channel.onmessage = (ev) => {
      if (typeof ev.data === "string") this.msgCb?.(ev.data);
    };
    channel.onclose = () => this.closeCb?.();
    pc.onconnectionstatechange = () => this.handleState(pc.connectionState);
  }

  private handleState(state: RTCPeerConnectionState): void {
    if (state === "connected") this.onState?.("connected");
    else if (state === "disconnected") this.onState?.("disconnected");
    else if (state === "failed") {
      this.onState?.("failed");
      this.closeCb?.(); // treat a failed link as a departed peer
    }
  }

  send(data: string): void {
    if (this.channel.readyState === "open") this.channel.send(data);
  }

  onMessage(cb: (d: string) => void): void {
    this.msgCb = cb;
  }

  onClose(cb: () => void): void {
    this.closeCb = cb;
  }

  close(): void {
    try {
      this.channel.close();
    } finally {
      this.pc.close();
      this.onState?.("closed");
    }
  }
}

/**
 * Resolves the Connection once the data channel opens; rejects (and closes the
 * peer connection) on ICE/connection failure or if it does not open in time. No
 * path leaves this promise pending.
 */
function connectionWhenOpen(
  peerId: string,
  channel: RTCDataChannel,
  pc: RTCPeerConnection,
  opts: RtcOptions,
): Promise<Connection> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const cleanup = (): void => {
      clearTimeout(timer);
      pc.removeEventListener("connectionstatechange", onConn);
      pc.removeEventListener("iceconnectionstatechange", onIce);
    };
    const ok = (): void => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(new RtcConnection(peerId, channel, pc, opts.onState));
    };
    const fail = (why: string): void => {
      if (settled) return;
      settled = true;
      cleanup();
      opts.onState?.("failed");
      pc.close();
      reject(new Error(why));
    };
    const onConn = (): void => {
      if (FATAL_STATES.has(pc.connectionState)) fail(`rtc: connection ${pc.connectionState}`);
    };
    const onIce = (): void => {
      if (FATAL_STATES.has(pc.iceConnectionState)) fail(`rtc: ICE ${pc.iceConnectionState}`);
    };
    const timer = setTimeout(() => fail("rtc: connection timed out before the channel opened"), opts.connectTimeoutMs ?? DEFAULT_CONNECT_TIMEOUT_MS);
    if (channel.readyState === "open") {
      ok();
      return;
    }
    channel.onopen = ok;
    pc.addEventListener("connectionstatechange", onConn);
    pc.addEventListener("iceconnectionstatechange", onIce);
  });
}

/** The initiating side: shares an offer blob, then accepts the answer blob. */
export interface OfferSession {
  /** Base64 offer blob to hand to the peer (e.g. as a QR code). */
  offerBlob(): Promise<string>;
  /** Consumes the peer's base64 answer blob; resolves when the channel is open. */
  acceptAnswer(answerBlob: string): Promise<Connection>;
  /** Tears down the peer connection if the invite is abandoned before it opens. */
  close(): void;
}

/**
 * Creates the offerer session for peer `selfId`. `payload` (the host's table
 * config) is embedded in the offer blob so the joiner builds its core from the
 * host's stakes/stack rather than a divergent default (issue #28 fix).
 */
export function createOffer(
  selfId: string,
  opts: RtcOptions = {},
  payload?: Record<string, unknown>,
): OfferSession {
  const pc = new RTCPeerConnection(rtcConfig(opts));
  const channel = pc.createDataChannel(CHANNEL_LABEL, { ordered: true });
  opts.onState?.("connecting");

  return {
    async offerBlob(): Promise<string> {
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await whenGathered(pc, opts.gatherTimeoutMs ?? DEFAULT_GATHER_TIMEOUT_MS);
      return encodeBlob({ peerId: selfId, sdp: pc.localDescription!, payload });
    },
    async acceptAnswer(answerBlob: string): Promise<Connection> {
      const blob = decodeBlob(answerBlob);
      await pc.setRemoteDescription(blob.sdp);
      return connectionWhenOpen(blob.peerId, channel, pc, opts);
    },
    close(): void {
      pc.close();
    },
  };
}

/** The joining side: consumes an offer blob and produces an answer blob. */
export interface AnswerSession {
  /** Base64 answer blob to hand back to the offerer. */
  answerBlob(): Promise<string>;
  /** Resolves when the data channel opens; rejects on failure/timeout. */
  connection(): Promise<Connection>;
  /** The host's embedded config payload (or undefined if the host sent none). */
  readonly offerPayload: Record<string, unknown> | undefined;
  /** The offerer's (host's) peer id, decoded from the invite blob. */
  readonly offerPeerId: string;
  /** Tears down the peer connection if the join is abandoned. */
  close(): void;
}

/** Creates the answerer session for peer `selfId` from a received offer blob. */
export async function acceptOffer(
  selfId: string,
  offerBlob: string,
  opts: RtcOptions = {},
): Promise<AnswerSession> {
  const blob = decodeBlob(offerBlob);
  const pc = new RTCPeerConnection(rtcConfig(opts));
  opts.onState?.("connecting");
  const remoteId = blob.peerId;
  const channelReady = new Promise<Connection>((resolve, reject) => {
    pc.ondatachannel = (ev) => {
      connectionWhenOpen(remoteId, ev.channel, pc, opts).then(resolve, reject);
    };
  });
  await pc.setRemoteDescription(blob.sdp);
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  await whenGathered(pc, opts.gatherTimeoutMs ?? DEFAULT_GATHER_TIMEOUT_MS);

  return {
    offerPayload: blob.payload,
    offerPeerId: remoteId,
    async answerBlob(): Promise<string> {
      return encodeBlob({ peerId: selfId, sdp: pc.localDescription! });
    },
    connection(): Promise<Connection> {
      return channelReady;
    },
    close(): void {
      pc.close();
    },
  };
}
