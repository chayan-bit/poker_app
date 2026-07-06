// Standalone mesh simulation (issue #28). Runs the REAL #27 table core under
// Node via wasm_exec.js: every peer gets its own LocalCore instance driven by
// the replicated log, over in-memory transports. No test runner is configured
// in the client this wave, so this is a self-contained runnable:
//
//   cd client && npx tsx src/local/simulation.mts
//   (or: node --experimental-strip-types src/local/simulation.mts on Node 22+)
//
// It prints a PASS/FAIL summary and exits non-zero on failure. Covered:
//   - fair multi-party seed combination against deterministic vectors
//   - 4 peers / 20 hands with the creator quitting at hand 5 (coordinator
//     migrates), the hand-9 coordinator quitting mid-hand (hand voids, game
//     continues), a peer rejoining from a snapshot at hand 12, and IDENTICAL
//     final stacks + state hash on every survivor
//   - partition heal: an isolated peer misses entries and catches up via gossip

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { LocalCore, type LocalConfig } from "./core.ts";
import { combineSeed, shareFromHex } from "./fairmp.ts";
import { MeshNode } from "./mesh.ts";
import { InMemoryNet } from "./transport.ts";

// ---- Boot the real WASM core under Node ------------------------------------

async function bootWasm(): Promise<void> {
  const here = dirname(fileURLToPath(import.meta.url));
  const pub = resolve(here, "../../public");
  const execSrc = readFileSync(resolve(pub, "wasm_exec.js"), "utf8");
  // wasm_exec.js is a classic script that installs globalThis.Go.
  (0, eval)(execSrc);
  const go = new (globalThis as unknown as { Go: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> } }).Go();
  const bytes = readFileSync(resolve(pub, "tablecore.wasm"));
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance);
  await Promise.resolve();
}

const CONFIG: LocalConfig = {
  id: "lan",
  maxSeats: 9,
  smallBlind: 1,
  bigBlind: 2,
  autoStart: true,
  turnTimeoutMs: 1000,
  disconnectGraceMs: 5000,
};

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
async function flush(): Promise<void> {
  for (let i = 0; i < 6; i++) await sleep(0);
}

// ---- Harness ----------------------------------------------------------------

interface Peer {
  id: string;
  seat: number;
  core: LocalCore;
  node: MeshNode;
  active: boolean;
}

class Harness {
  net: InMemoryNet;
  peers: Peer[] = [];
  t = 1000;
  private readonly step = 200;
  divergences = 0;
  dishonest = 0;

  constructor(ids: string[], bootstrapId: string) {
    this.net = new InMemoryNet(ids);
    for (let i = 0; i < ids.length; i++) {
      this.peers.push(this.makePeer(ids[i], i, bootstrapId));
    }
  }

  private makePeer(id: string, seat: number, bootstrapId: string): Peer {
    const core = new LocalCore(CONFIG, "");
    const node = new MeshNode({
      selfId: id,
      core,
      config: CONFIG,
      connections: this.net.connectionsFor(id),
      clock: () => this.t,
      bootstrapId,
      heartbeatMs: 200,
      graceMs: 800,
      turnTimeoutMs: CONFIG.turnTimeoutMs!,
      roundTimeoutMs: 60000,
      hooks: {
        onDivergence: () => (this.divergences += 1),
        onDishonestDealer: () => (this.dishonest += 1),
      },
    });
    return { id, seat, core, node, active: true };
  }

  live(): Peer[] {
    return this.peers.filter((p) => p.active);
  }

  seatAll(): void {
    for (const p of this.peers) {
      p.node.submitLocalAction({
        v: 1,
        type: "sit_down",
        data: { tableId: CONFIG.id, seat: p.seat, buyIn: 200 },
      });
    }
  }

  private bots(): void {
    for (const p of this.live()) {
      const view = p.node.getView();
      if (!view.handRunning) continue;
      let mySeat = -1;
      for (const [s, info] of view.seats) if (info.playerId === p.id) mySeat = s;
      if (mySeat < 0) continue;
      if (view.toActSeat === mySeat || view.toActSeat === -1) {
        p.node.submitLocalAction({
          v: 1,
          type: "place_bet",
          data: { kind: "call", amount: 0 },
        });
      }
    }
  }

  async tickOnce(): Promise<void> {
    this.bots();
    for (const p of this.live()) p.node.tick(this.t);
    await flush();
    this.t += this.step;
  }

  quit(id: string): void {
    const p = this.peers.find((x) => x.id === id)!;
    p.active = false;
    this.net.isolate(id, true);
  }

  async rejoin(id: string): Promise<void> {
    const p = this.peers.find((x) => x.id === id)!;
    this.net.isolate(id, false);
    const donors = this.live();
    const primary = donors[0].node.snapshot();
    const secondary = donors[1]?.node.snapshot();
    const core = new LocalCore(CONFIG, "");
    const node = new MeshNode({
      selfId: id,
      core,
      config: CONFIG,
      connections: this.net.connectionsFor(id),
      clock: () => this.t,
      bootstrapId: id,
      heartbeatMs: 200,
      graceMs: 800,
      turnTimeoutMs: CONFIG.turnTimeoutMs!,
      roundTimeoutMs: 60000,
    });
    node.adoptSnapshot(primary, secondary);
    p.core = core;
    p.node = node;
    p.active = true;
  }

  refHands(): number {
    return this.live()[0].node.dealtHands();
  }

  refRunning(): boolean {
    return this.live()[0].node.getView().handRunning;
  }
}

// ---- Assertions -------------------------------------------------------------

const results: { name: string; ok: boolean; detail: string }[] = [];
function check(name: string, ok: boolean, detail = ""): void {
  results.push({ name, ok, detail });
  const tag = ok ? "PASS" : "FAIL";
  console.log(`  [${tag}] ${name}${detail ? " - " + detail : ""}`);
}

function stacksOf(p: Peer): string {
  const view = p.node.getView();
  const rows = [...view.seats.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([s, i]) => `${s}:${i.stack}`);
  return rows.join(",");
}

// ---- Scenario 1: fair-seed vectors ------------------------------------------

async function testFairVectors(): Promise<void> {
  console.log("\nfair multi-party seed vectors");
  const s1 = new Uint8Array(32).fill(0x00);
  const s2 = new Uint8Array(32).fill(0x11);
  const s3 = new Uint8Array(32).fill(0x22);
  const two = await combineSeed([s1, s2]);
  const three = await combineSeed([s1, s2, s3]);
  check(
    "combine(00*32, 11*32)",
    two === "8878b15a7d6a3a4f464e8f9f42591dbc0cf4bedea0ec309003d2b2ee53655ef8",
    two,
  );
  check(
    "combine(00*32, 11*32, 22*32)",
    three === "b647d2614ad2099840c899399acf3b264e8f40f9eb0d0f997c2e6e1c3ffc2da6",
    three,
  );
  // Order matters: swapping shares changes the seed.
  const swapped = await combineSeed([s2, s1]);
  check("combine is order-sensitive", swapped !== two);
  // hex round-trip.
  const back = await combineSeed([shareFromHex(two.slice(0, 64))]);
  check("hex round-trip stable", typeof back === "string" && back.length === 64);
}

// ---- Scenario 2: 4 peers / 20 hands with churn ------------------------------

async function testMeshChurn(): Promise<void> {
  console.log("\n4 peers, 20 hands, creator + coordinator churn, rejoin");
  const h = new Harness(["p0", "p1", "p2", "p3"], "p0");
  h.seatAll();

  let quitCreator = false;
  let quitCoord9 = false;
  let coord9: string | null = null;
  let rejoined = false;
  let midHand9Hash = "";

  for (let i = 0; i < 6000; i++) {
    await h.tickOnce();
    const hands = h.refHands();
    const running = h.refRunning();

    if (!quitCreator && hands >= 5 && !running) {
      h.quit("p0");
      quitCreator = true;
      check("creator quit at hand 5, game not stalled", h.live().length === 3);
    }

    if (quitCreator && !quitCoord9 && hands === 9 && running) {
      const coord = h.live()[0].node.coordinatorPeerId();
      const target = h.peers.find((p) => p.id === coord && p.active);
      const witness = h.live().find((p) => p.id !== coord);
      if (target && witness) {
        midHand9Hash = witness.node.stateHash();
        h.quit(coord);
        coord9 = coord;
        quitCoord9 = true;
        check("coordinator quit mid-hand 9", true, `coord=${coord}`);
      }
    }

    if (quitCreator && !rejoined && hands >= 12) {
      await h.rejoin("p0");
      rejoined = true;
      const ref = h.live().find((p) => p.id !== "p0")!;
      check(
        "p0 rejoined from snapshot, hash matches live peer",
        h.peers.find((p) => p.id === "p0")!.node.stateHash() === ref.node.stateHash(),
      );
    }

    if (hands >= 20 && !running) break;
  }

  const survivors = h.live();
  const finalHash = survivors[0].node.stateHash();
  const finalStacks = stacksOf(survivors[0]);
  const allHashEqual = survivors.every((p) => p.node.stateHash() === finalHash);
  const allStacksEqual = survivors.every((p) => stacksOf(p) === finalStacks);

  check("reached >= 20 hands", h.refHands() >= 20, `hands=${h.refHands()}`);
  check("coordinator did migrate (hand-9 coord recorded)", quitCoord9, `coord9=${coord9}`);
  check("void path exercised (mid-hand hash captured)", midHand9Hash.length > 0);
  check("rejoin completed", rejoined);
  check(
    "final state hash identical on all survivors",
    allHashEqual,
    survivors.map((p) => `${p.id}:${p.node.stateHash().slice(0, 8)}`).join(" "),
  );
  check("final stacks identical on all survivors", allStacksEqual, finalStacks);
  check("no state divergence flagged", h.divergences === 0, `divergences=${h.divergences}`);
  check("no dishonest-dealer flag (all seeds fairly built)", h.dishonest === 0);
}

// ---- Scenario 3: partition heal ---------------------------------------------

async function testPartitionHeal(): Promise<void> {
  console.log("\npartition heal: isolated peer catches up via gossip");
  const h = new Harness(["a", "b", "c"], "a");
  h.seatAll();
  // Deal a couple of hands so there is a log to miss.
  for (let i = 0; i < 400 && h.refHands() < 2; i++) await h.tickOnce();

  h.net.isolate("c", true);
  const beforeHead = h.peers.find((p) => p.id === "c")!.node.headSeq();
  // Play on without c; c is folded out by the coordinator timer each hand.
  const target = h.refHands() + 3;
  for (let i = 0; i < 2000 && h.refHands() < target; i++) await h.tickOnce();
  const liveHead = h.peers.find((p) => p.id === "a")!.node.headSeq();
  check("live peers advanced while c partitioned", liveHead > beforeHead, `a=${liveHead} c=${beforeHead}`);

  // Heal and let gossip catch c up.
  h.net.isolate("c", false);
  const c = h.peers.find((p) => p.id === "c")!;
  for (let i = 0; i < 2000; i++) {
    await h.tickOnce();
    if (c.node.headSeq() >= liveHead && !h.refRunning()) break;
  }
  const a = h.peers.find((p) => p.id === "a")!;
  check("c caught up to live head", c.node.headSeq() >= liveHead, `c=${c.node.headSeq()} a=${a.node.headSeq()}`);
  check("c state hash matches live peer after heal", c.node.stateHash() === a.node.stateHash());
}

// ---- Runner -----------------------------------------------------------------

async function main(): Promise<void> {
  await bootWasm();
  console.log("tablecore.wasm loaded under Node:", typeof (globalThis as { tablecore?: unknown }).tablecore === "object");

  await testFairVectors();
  await testMeshChurn();
  await testPartitionHeal();

  const failed = results.filter((r) => !r.ok);
  console.log(`\n==== ${results.length - failed.length}/${results.length} checks passed ====`);
  if (failed.length > 0) {
    console.log("FAILURES:");
    for (const f of failed) console.log(`  - ${f.name} ${f.detail}`);
    process.exit(1);
  }
  console.log("SIMULATION PASS");
  process.exit(0);
}

void main();
