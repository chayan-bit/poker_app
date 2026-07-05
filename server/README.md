# pokerd - poker_app game server

A single lightweight Go binary. No runtime, no VM, no GC pauses that matter:
one static executable serving the authoritative poker engine over WebSockets.

## Why Go

The server sits in the latency hot path, so it must add as little as possible on
top of the network RTT. Go was chosen over the original Python/FastAPI sketch
because:

- **Compiled, no interpreter** - predictable single-digit-microsecond handling
  per message; no GIL, no per-request interpreter overhead.
- **Goroutine-per-connection** - tens of thousands of concurrent WebSocket
  connections on modest hardware, each table an isolated goroutine that is the
  single writer of its own state (so the pure engine needs no locks).
- **Sub-millisecond GC** - Go's low-latency collector keeps tail latency flat,
  which is exactly the "no lag" property the product is built on.
- **One static binary** - trivial to deploy to regional edge gateways for the
  sub-100 ms RTT target.

## Layout

```
server/
├── cmd/pokerd/main.go        # entrypoint: HTTP + /ws, graceful shutdown
└── internal/
    ├── engine/               # PURE poker core: no I/O, no randomness, no clock
    │   ├── card.go           # card/deck value types
    │   ├── eval.go           # 5..7-card hand evaluator + total ordering
    │   ├── state.go          # immutable HandState + Apply(action) state machine
    │   └── pots.go           # main/side-pot construction and distribution
    ├── fair/                 # provably-fair commit-reveal shuffle
    ├── protocol/             # wire messages (single source of truth for client)
    ├── table/                # live table goroutine + registry (public/private)
    ├── ws/                   # WebSocket edge: reader/writer, routing
    ├── auth/                 # guest tokens -> full accounts (scaffold)
    └── economy/              # fake-money ledger: refills, floor (scaffold)
```

## Design invariants

- The **engine is pure**: `(HandState, Action) -> HandState`, no mutation, no
  side effects. The shuffled deck is an input produced by `fair`.
- The **server is authoritative**: illegal actions are rejected here, never on
  the client.
- **Chips are integers** (`engine.Chips`), never floats.
- Randomness comes only from `fair.NewSeed()` (OS CSPRNG); `math/rand` is banned
  in game paths.

## Run

```sh
cd server
go run ./cmd/pokerd        # listens on :8080, POKERD_ADDR to override
go test ./...              # engine + fair are the critical suites
```

## Status

Scaffold. Engine hand-evaluation, provably-fair shuffle, and the betting state
machine core are implemented and tested. `table`, `ws`, `auth`, `economy` have
working structure with `TODO` markers where persistence (PostgreSQL/Redis) and
the full command handlers plug in.
