// Module-level network-mode flag. When the app enters "Play with friends
// offline" (nearby) mode, no cloud API/WS call is legal: the whole session runs
// peer-to-peer over the mesh. api.ts consults this flag and throws loudly in dev
// if a REST call slips through while offline, so a stray cloud dependency in the
// nearby path is caught at the source instead of silently hitting a server.
//
// This is a single boolean, not a store: it must be readable from plain modules
// (api.ts) with no React/zustand dependency, and there is exactly one active
// network mode per app instance at a time.

let offline = false;

/** Enters or leaves offline (nearby) mode. Called when the nearby flow mounts
 *  and unmounts. */
export function setOfflineMode(active: boolean): void {
  offline = active;
}

/** True while the app is in peer-to-peer nearby mode; no cloud calls allowed. */
export function isOfflineMode(): boolean {
  return offline;
}
