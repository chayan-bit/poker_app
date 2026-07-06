// Adapter: raw local-server peer pipe -> mesh Connection (issue #28).
//
// The mesh (client/src/local/transport.ts) consumes Connection objects. This
// factory turns the plugin's peer-keyed byte pipe into one Connection per peer,
// so the app can feed native LAN sockets to the exact same MeshNode it feeds
// in-memory or WebRTC transports. The Connection shape is re-declared here (not
// imported) to keep the plugin free of any app dependency; it must stay in sync
// with transport.ts.

import type { LocalServerPlugin, PluginListenerHandle } from "./definitions";

/** Mirror of client/src/local/transport.ts Connection. Keep in sync. */
export interface Connection {
  readonly peerId: string;
  send(data: string): void;
  onMessage(cb: (data: string) => void): void;
  onClose(cb: () => void): void;
  close(): void;
}

/**
 * Bridges one plugin instance to Connection objects. Call attach() once after
 * start(); it fans the plugin's `message`/`connection` events out to per-peer
 * Connection handlers and returns a `connectionFor(peerId)` factory plus a
 * `dispose()` that removes the listeners.
 */
export async function bridgeConnections(plugin: LocalServerPlugin): Promise<{
  connectionFor(peerId: string): Connection;
  dispose(): Promise<void>;
}> {
  const messageCbs = new Map<string, (d: string) => void>();
  const closeCbs = new Map<string, () => void>();
  const handles: PluginListenerHandle[] = [];

  handles.push(
    await plugin.addListener("message", (e) => messageCbs.get(e.peerId)?.(e.data)),
  );
  handles.push(
    await plugin.addListener("connection", (e) => {
      if (e.state === "closed") closeCbs.get(e.peerId)?.();
    }),
  );

  const connectionFor = (peerId: string): Connection => ({
    peerId,
    send: (data: string) => void plugin.send({ peerId, data }),
    onMessage: (cb) => messageCbs.set(peerId, cb),
    onClose: (cb) => closeCbs.set(peerId, cb),
    close: () => void plugin.disconnect({ peerId }),
  });

  const dispose = async (): Promise<void> => {
    for (const h of handles) await h.remove();
    messageCbs.clear();
    closeCbs.clear();
  };

  return { connectionFor, dispose };
}
