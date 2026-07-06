// Web fallback for @feltpoker/local-server. Browsers cannot open a raw mDNS
// listener or bind a local TCP WebSocket server, so the browser mesh path uses
// WebRTC data channels (client/src/local/rtc.ts) instead. Every method here
// throws so a miswired web build fails loudly rather than silently no-op'ing.

import { WebPlugin } from "@capacitor/core";

import type {
  LocalServerPlugin,
  PluginListenerHandle,
  SendOptions,
  StartOptions,
  StartResult,
} from "./definitions";

const NATIVE_ONLY = "local-server: native only. Browser peers join via WebRTC (client/src/local/rtc.ts).";

export class LocalServerWeb extends WebPlugin implements LocalServerPlugin {
  async start(_options: StartOptions): Promise<StartResult> {
    throw this.unimplemented(NATIVE_ONLY);
  }
  async stop(): Promise<void> {
    throw this.unimplemented(NATIVE_ONLY);
  }
  async connect(_options: { peerId: string; host: string; port: number }): Promise<void> {
    throw this.unimplemented(NATIVE_ONLY);
  }
  async send(_options: SendOptions): Promise<void> {
    throw this.unimplemented(NATIVE_ONLY);
  }
  async disconnect(_options: { peerId: string }): Promise<void> {
    throw this.unimplemented(NATIVE_ONLY);
  }
  // The typed overloads live in definitions.ts; the web stub accepts the base
  // signature and rejects at runtime.
  async addListener(): Promise<PluginListenerHandle> {
    throw this.unimplemented(NATIVE_ONLY);
  }
}
