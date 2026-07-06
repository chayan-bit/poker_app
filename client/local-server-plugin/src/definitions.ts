// Public interface of the @feltpoker/local-server Capacitor plugin (issue #28).
//
// Every native peer runs one instance: it advertises itself over mDNS
// (_feltpoker._tcp), browses for other peers, and hosts a local WebSocket
// endpoint. Discovered peers connect over those sockets to form the LAN mesh.
// The plugin is a raw byte pipe keyed by peer id; the replicated state machine
// (client/src/local/mesh.ts) sits entirely above it, so this interface stays
// deliberately transport-level and knows nothing about the poker protocol.
//
// The shapes below intentionally mirror client/src/local/transport.ts Connection
// semantics (peer id + string frames + open/message/close), so a thin adapter
// turns "a discovered + connected peer" into a mesh Connection (see README).

export interface StartOptions {
  /** Stable id this device announces and is addressed by on the mesh. */
  peerId: string;
  /** Bonjour service type. Defaults to "_feltpoker._tcp". */
  serviceType?: string;
  /** TCP port for the local WebSocket listener. 0 asks the OS to pick one. */
  port?: number;
  /** Human-visible instance name for the advertised service. */
  displayName?: string;
}

export interface StartResult {
  /** The port actually bound (useful when `port` was 0). */
  port: number;
}

export interface SendOptions {
  /** Target peer id (as discovered via the `peerDiscovered` event). */
  peerId: string;
  /** UTF-8 payload; the mesh sends JSON strings. */
  data: string;
}

export interface PeerDiscoveredEvent {
  peerId: string;
  host: string;
  port: number;
  displayName?: string;
}

export interface PeerConnectionEvent {
  peerId: string;
  /** "open" when a duplex socket is ready, "closed" when it drops. */
  state: "open" | "closed";
}

export interface MessageEvent {
  peerId: string;
  data: string;
}

export interface PluginListenerHandle {
  remove(): Promise<void>;
}

export interface LocalServerPlugin {
  /** Begin advertising, browsing, and listening. Idempotent per process. */
  start(options: StartOptions): Promise<StartResult>;
  /** Stop everything and drop all sockets. */
  stop(): Promise<void>;
  /** Dial a discovered peer's local WebSocket (no-op if already connected). */
  connect(options: { peerId: string; host: string; port: number }): Promise<void>;
  /** Send one frame to a connected peer. */
  send(options: SendOptions): Promise<void>;
  /** Close a single peer connection. */
  disconnect(options: { peerId: string }): Promise<void>;

  addListener(eventName: "peerDiscovered", listener: (e: PeerDiscoveredEvent) => void): Promise<PluginListenerHandle>;
  addListener(eventName: "connection", listener: (e: PeerConnectionEvent) => void): Promise<PluginListenerHandle>;
  addListener(eventName: "message", listener: (e: MessageEvent) => void): Promise<PluginListenerHandle>;
  removeAllListeners(): Promise<void>;
}
