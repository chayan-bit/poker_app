// Peer-id encoding for nearby play. A mesh peer id must be globally unique and
// stable, and it is the only identifier that travels with a seat across the
// wire (seat_update carries playerId, never a display name). So we carry the
// chosen display name INSIDE the peer id and decode it for the felt. This keeps
// names propagating through the full mesh (and snapshot replay) with zero mesh
// protocol changes.

const SEP = "~";

/** Builds a stable, unique peer id that embeds a sanitized display name. */
export function encodePeerId(name: string): string {
  const clean = name.replace(new RegExp(SEP, "g"), " ").trim().slice(0, 24) || "Player";
  const rand = Math.random().toString(36).slice(2, 8);
  return `${clean}${SEP}${rand}`;
}

/** Recovers the display name from a peer id produced by encodePeerId. */
export function displayName(peerId: string): string {
  const i = peerId.lastIndexOf(SEP);
  return i > 0 ? peerId.slice(0, i) : peerId;
}
