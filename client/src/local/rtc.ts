// WebRTC data-channel transport with serverless QR signaling (issue #28).
//
// A browser guest joins the mesh with no signaling server: the offerer produces
// a compact base64 blob (SDP + gathered ICE candidates), the answerer consumes
// it and produces an answer blob, and the two are exchanged out of band (a QR
// code each way; QR rendering itself is issue #29 and stays out of here). On a
// LAN, host candidates alone connect, so no STUN/TURN is configured.
//
// This module only manufactures the blob strings and the Connection; it never
// touches the DOM. RTCPeerConnection / RTCDataChannel are ambient DOM types, so
// this compiles under the app tsconfig and runs in any browser.

import type { Connection } from "./transport.ts";

/** Empty ICE config: LAN host candidates only, no STUN/TURN round trips. */
const LAN_RTC_CONFIG: RTCConfiguration = { iceServers: [] };
const CHANNEL_LABEL = "feltpoker-mesh";

interface SignalBlob {
  peerId: string;
  sdp: RTCSessionDescriptionInit;
}

function encodeBlob(blob: SignalBlob): string {
  return btoa(JSON.stringify(blob));
}

function decodeBlob(text: string): SignalBlob {
  return JSON.parse(atob(text)) as SignalBlob;
}

/** Resolves once ICE gathering completes so the SDP carries all host candidates. */
function whenGathered(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    const check = () => {
      if (pc.iceGatheringState === "complete") {
        pc.removeEventListener("icegatheringstatechange", check);
        resolve();
      }
    };
    pc.addEventListener("icegatheringstatechange", check);
  });
}

/** Wraps an RTCDataChannel as a mesh Connection. */
class RtcConnection implements Connection {
  readonly peerId: string;
  private msgCb: ((d: string) => void) | null = null;
  private closeCb: (() => void) | null = null;

  constructor(peerId: string, private readonly channel: RTCDataChannel, private readonly pc: RTCPeerConnection) {
    this.peerId = peerId;
    channel.onmessage = (ev) => {
      if (typeof ev.data === "string") this.msgCb?.(ev.data);
    };
    channel.onclose = () => this.closeCb?.();
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
    this.channel.close();
    this.pc.close();
  }
}

/** Resolves the Connection once the data channel opens. */
function connectionWhenOpen(peerId: string, channel: RTCDataChannel, pc: RTCPeerConnection): Promise<Connection> {
  return new Promise((resolve) => {
    if (channel.readyState === "open") {
      resolve(new RtcConnection(peerId, channel, pc));
      return;
    }
    channel.onopen = () => resolve(new RtcConnection(peerId, channel, pc));
  });
}

/** The initiating side: shares an offer blob, then accepts the answer blob. */
export interface OfferSession {
  /** Base64 offer blob to hand to the peer (e.g. as a QR code). */
  offerBlob(): Promise<string>;
  /** Consumes the peer's base64 answer blob; resolves when the channel is open. */
  acceptAnswer(answerBlob: string): Promise<Connection>;
}

/** Creates the offerer session for peer `selfId` joining the mesh. */
export function createOffer(selfId: string): OfferSession {
  const pc = new RTCPeerConnection(LAN_RTC_CONFIG);
  const channel = pc.createDataChannel(CHANNEL_LABEL, { ordered: true });
  let remoteId = "";

  return {
    async offerBlob(): Promise<string> {
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await whenGathered(pc);
      return encodeBlob({ peerId: selfId, sdp: pc.localDescription! });
    },
    async acceptAnswer(answerBlob: string): Promise<Connection> {
      const blob = decodeBlob(answerBlob);
      remoteId = blob.peerId;
      await pc.setRemoteDescription(blob.sdp);
      return connectionWhenOpen(remoteId, channel, pc);
    },
  };
}

/** The joining side: consumes an offer blob and produces an answer blob. */
export interface AnswerSession {
  /** Base64 answer blob to hand back to the offerer. */
  answerBlob(): Promise<string>;
  /** Resolves when the data channel opens. */
  connection(): Promise<Connection>;
}

/** Creates the answerer session for peer `selfId` from a received offer blob. */
export async function acceptOffer(selfId: string, offerBlob: string): Promise<AnswerSession> {
  const pc = new RTCPeerConnection(LAN_RTC_CONFIG);
  const blob = decodeBlob(offerBlob);
  const remoteId = blob.peerId;
  const channelReady = new Promise<Connection>((resolve) => {
    pc.ondatachannel = (ev) => {
      void connectionWhenOpen(remoteId, ev.channel, pc).then(resolve);
    };
  });
  await pc.setRemoteDescription(blob.sdp);
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  await whenGathered(pc);

  return {
    async answerBlob(): Promise<string> {
      return encodeBlob({ peerId: selfId, sdp: pc.localDescription! });
    },
    connection(): Promise<Connection> {
      return channelReady;
    },
  };
}
