// Split-brain guard tests for MeshNode: the deterministic successor math says
// WHO would coordinate (coordinator.ts, tested separately); the guard says
// WHETHER to act on it - only when at least one OTHER live peer is visible, so
// a peer partitioned alone never sequences divergent hands.

import { beforeEach, describe, expect, it } from "vitest";
import { MeshNode } from "./mesh.ts";
import type { CoreLike } from "./meshtypes.ts";
import type { Connection } from "./transport.ts";
import type { MeshMsg } from "./wire.ts";

/** A do-nothing core: the guard never touches game state. */
function fakeCore(): CoreLike {
  return {
    submit: () => ({}),
    tick: () => ({}),
    stateHash: () => "hash",
    voidHand: () => ({}),
    setSeed: () => {},
  };
}

/** A recording connection whose inbound side the test drives directly. */
class FakeConn implements Connection {
  readonly sent: string[] = [];
  private msgCb: ((data: string) => void) | null = null;

  constructor(readonly peerId: string) {}

  send(data: string): void {
    this.sent.push(data);
  }

  onMessage(cb: (data: string) => void): void {
    this.msgCb = cb;
  }

  onClose(): void {}

  close(): void {}

  deliver(msg: MeshMsg): void {
    this.msgCb?.(JSON.stringify(msg));
  }
}

const GRACE_MS = 2000;

describe("MeshNode split-brain guard", () => {
  let now: number;
  let conn: FakeConn;
  let node: MeshNode;

  beforeEach(() => {
    now = 1_000;
    conn = new FakeConn("p2");
    node = new MeshNode({
      selfId: "p1",
      core: fakeCore(),
      config: { id: "t1", smallBlind: 1, bigBlind: 2 },
      connections: [conn],
      clock: () => now,
      bootstrapId: "p1",
      graceMs: GRACE_MS,
    });
  });

  function heartbeatFromP2(): void {
    conn.deliver({ t: "heartbeat", from: "p2", head: 0, nowMs: now, coordSeat: -1 });
  }

  it("names the bootstrap peer as coordinator before any seat is occupied", () => {
    expect(node.coordinatorPeerId()).toBe("p1");
  });

  it("does not assume the coordinator role while no other live peer is visible", () => {
    // p1 is the designated coordinator but is alone: acting would split-brain.
    expect(node.coordinatorPeerId()).toBe("p1");
    expect(node.isCoordinator()).toBe(false);
  });

  it("assumes the coordinator role once one other live peer is visible", () => {
    heartbeatFromP2();
    expect(node.isCoordinator()).toBe(true);
  });

  it("keeps the role while the peer's heartbeat stays within the grace window", () => {
    heartbeatFromP2();
    now += GRACE_MS; // exactly at the edge is still alive (<=)
    expect(node.isCoordinator()).toBe(true);
  });

  it("drops the coordinator role when the last other peer goes silent past grace", () => {
    heartbeatFromP2();
    now += GRACE_MS + 1;
    expect(node.isCoordinator()).toBe(false);
  });

  it("re-assumes the role when the partition heals and heartbeats resume", () => {
    heartbeatFromP2();
    now += GRACE_MS + 1;
    expect(node.isCoordinator()).toBe(false);
    heartbeatFromP2();
    expect(node.isCoordinator()).toBe(true);
  });

  it("treats a hello frame like a heartbeat for liveness", () => {
    conn.deliver({ t: "hello", from: "p2" });
    expect(node.isCoordinator()).toBe(true);
  });
});
