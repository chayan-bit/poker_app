// Entry point for @feltpoker/local-server (issue #28).
//
// registerPlugin wires the JS proxy to the native LocalServer implementations
// (iOS/Android) and falls back to the web stub (which throws "native only").
// See adapter.ts for turning this raw peer pipe into mesh Connection objects.

import { registerPlugin } from "@capacitor/core";

import type { LocalServerPlugin } from "./definitions";

const LocalServer = registerPlugin<LocalServerPlugin>("LocalServer", {
  web: () => import("./web").then((m) => new m.LocalServerWeb()),
});

export * from "./definitions";
export * from "./adapter";
export { LocalServer };
