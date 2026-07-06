# @feltpoker/local-server

Capacitor plugin for offline LAN poker (poker_app issue #28).
Every native peer runs one instance: it advertises itself over mDNS (`_feltpoker._tcp`), browses for other peers, and hosts its own local WebSocket endpoint.
There is no privileged host - this plugin is the discovery + byte-pipe layer under the replicated state machine in `client/src/local/mesh.ts`.

## Status: self-contained skeleton

This package is complete at the interface, bridge, and adapter layers.
The native networking bodies (iOS `NWListener`/Bonjour, Android `NsdManager`/WebSocket) are honest, well-structured stubs with precise `TODO(device)` markers, because validating mDNS and LAN sockets requires two physical devices on the same Wi-Fi, which cannot be exercised in this environment.

The plugin is NOT yet wired into `client/package.json` or the `client/ios` / `client/android` projects - those are owned by another workstream this wave.
To integrate later: `npm i ./local-server-plugin` from `client/`, then `npx cap sync`.

## Layout

- `src/definitions.ts` - the `LocalServerPlugin` TypeScript interface (transport-level: peer ids + string frames + discovery/connection events).
- `src/web.ts` - web fallback; every method throws `native only` (browsers join via WebRTC, see `client/src/local/rtc.ts`).
- `src/index.ts` - `registerPlugin` entry point.
- `src/adapter.ts` - `bridgeConnections()` turns the plugin's peer pipe into mesh `Connection` objects (mirrors `client/src/local/transport.ts`).
- `ios/Sources/LocalServerPlugin/` - Capacitor `@objc` bridge + `LocalServer` (Network.framework).
- `android/src/main/java/com/feltpoker/localserver/` - Capacitor `@PluginMethod` bridge + `LocalServer` (NSD + WebSocket).
- `FeltpokerLocalServer.podspec`, `package.json`, `tsconfig.json` - packaging.

## How it plugs into the mesh

```ts
import { LocalServer, bridgeConnections } from "@feltpoker/local-server";
import { MeshNode } from "@/local/mesh";

await LocalServer.start({ peerId });
const bridge = await bridgeConnections(LocalServer);
await LocalServer.addListener("peerDiscovered", async (p) => {
  await LocalServer.connect(p);          // dial the discovered peer
  mesh.attach(bridge.connectionFor(p.peerId)); // hand the Connection to the mesh
});
```

The same `MeshNode` also accepts in-memory `Connection`s (tests) and WebRTC `Connection`s (browser guests), so game screens stay transport-blind.

## Trust model

Fun chips, friends, one LAN. The dealer for each hand (the coordinator) sees that hand's deck, exactly like a physical dealer; the role rotates with the button.
Fairness is enforced by post-hand verification, not prevention (see `client/src/local/fairmp.ts`).
Full mental poker (commutative encryption) is explicitly out of scope.
