# CLAUDE.md - poker_app

Working instructions for AI agents and contributors on this repo.
Extends the user's global CLAUDE.md; nothing here overrides the global Nix-only package policy, TDD, or commit conventions.

## What this project is

A cross-platform (web + iOS + Android) social poker app with fake money, public and private lobbies, and a friends layer.
North-star quality bar: **zero perceived lag**.
All product and design rationale lives in `docs/Design_suite.md`; read it before making UX, animation, or protocol decisions.

## Non-negotiables

- **Performance budget is a hard constraint, not a goal.**
  Input feedback under 100 ms, server action round-trip under 150 ms perceived (optimistic UI covers the rest), animations at 60 fps using transform/opacity only.
  Any PR that adds layout-thrashing animation, blocking spinners in the action path, or per-frame React re-renders of the table is wrong by definition.
- **The server is authoritative.**
  Clients never compute game outcomes; they render server-confirmed state and may only optimistically preview the local player's own action.
- **No dark patterns.**
  No ads, no purchase nags, no artificial losing streaks, no interstitials. This is the product's identity.
- **Provably fair shuffle is a protocol invariant.**
  Deck seed commit before the hand, reveal after; never ship a code path that deals without a committed seed.
- **Game logic is pure and mutation-free.**
  The engine is a pure state machine: `(state, action) -> new state`. No I/O, no randomness inside the engine; the shuffled deck is an input.

## Stack

- Client: React 19 + TypeScript + Vite + Tailwind; Capacitor wraps the same build for iOS/Android; Framer Motion only for non-table chrome, table animations are hand-rolled CSS transforms/WAAPI.
- Server: Go 1.26 + WebSockets (coder/websocket); a single static binary, goroutine-per-connection, engine as a pure package; PostgreSQL (accounts, hand history, economy); Redis (live table state, pub/sub, presence). Chosen over FastAPI because the server is in the latency hot path.
- Shared protocol: JSON messages under 1 KB, versioned, defined once in `server/internal/protocol/` (Go structs are the source of truth) and mirrored to client TS types.

## Testing expectations

- The poker engine (hand evaluation, pot math, side pots, state machine) requires exhaustive unit tests before any UI work touches it; side-pot math and split-pot edge cases are where poker apps silently rot.
- TDD per global rules, 80%+ coverage; engine target is ~100%.
- E2E: a scripted 3-client game (join, bet, fold, showdown, disconnect/reconnect) must pass before merging protocol changes.
- Latency regression: keep a benchmark that fails CI if action-ack round trip on localhost exceeds budget.

## Conventions specific to this repo

- Monorepo layout: `server/` (Go: `cmd/pokerd`, `internal/{engine,fair,protocol,table,ws,auth,economy}`), `client/` (design-tool-generated web client + Capacitor wrapper), `docs/`.
- Chips are integers (smallest unit = 1 chip); never floats anywhere in money paths.
- All randomness comes from the server CSPRNG via the seeded-shuffle module; `random`/`Math.random` are banned in game paths.
- WebSocket messages: past-tense events from server (`hand_dealt`, `bet_placed`), imperative commands from client (`place_bet`).
- Feature flags for anything experimental (fun features from Design_suite section 9) so the core table stays lean.
