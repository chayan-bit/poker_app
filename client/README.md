# poker_app — client

Client-side web app for **poker_app**, built to feel instant. React 19 +
TypeScript + Vite + Tailwind, wrapped with Capacitor for iOS/Android from the
same codebase. No backend here — the Go server is authoritative; this is UI,
state, animations, and the WebSocket client layer.

## Run

```bash
npm install          # if NODE_ENV=production is set globally, use:
                     # NODE_ENV=development npm install --include=dev
npm run dev          # fixture mode — a mock server drives a live demo hand
```

By default the client runs in **fixture mode**: `src/net/mockServer.ts` speaks
the exact same protocol as the real server, so the table is fully demoable with
no backend. To point at the real `pokerd`:

```bash
VITE_WS_URL=ws://localhost:8080/ws npm run dev
```

## Scripts

- `npm run dev` — Vite dev server
- `npm run build` — `tsc -b` + `vite build` (outputs `dist/`)
- `npm run typecheck` — strict type check, no emit
- `npm run cap:sync` — sync the web build into native shells

## Capacitor (iOS / Android)

`capacitor.config.ts` points `webDir` at `dist/`.

```bash
npm run build
npx cap add ios
npx cap add android
npx cap sync
```

## The one hard rule: it feels instant

- **Input feedback < 100 ms, always local.** `gameStore.act()` renders the
  local player's action optimistically (tagged `pending`) before the server
  confirms. No spinner in the action path.
- **Optimistic UI with reconciliation.** The server's `bet_placed` clears the
  pending flag; an `error` rolls it back with a subtle action-bar shake
  (`.rollback-shake`) — never a modal. Opponents render only on server events.
- **Table animations are hand-rolled transform/opacity** via the Web Animations
  API (`src/motion/primitives.ts`) — not Framer Motion, not rAF loops — so the
  main thread stays free for the socket. Framer Motion is used only for
  non-table chrome (lobby transitions).
- **The table is a stable scene graph.** Seats/board/pot are persistent DOM
  nodes that MOVE via transforms; re-render is per discrete event, never per
  frame.
- **`prefers-reduced-motion`** collapses all motion to 80 ms fades; an in-app
  toggle (Settings) forces the same.
- **Code-split**: the table route is the priority bundle; lobby, settings,
  replayer, fairness and auth are lazy chunks.

## Structure

```
src/
  net/         protocol.ts (mirrors server/internal/protocol/messages.go),
               client.ts (WS: envelope, seq-gap -> resync, reconnect),
               mockServer.ts (fixture server, same interface)
  store/       gameStore.ts (render-side state, optimistic layer),
               settingsStore.ts (persisted settings + theme classes)
  motion/      primitives.ts (deal, flip, chip-bet, pot-merge, pot-push,
               fold-muck, timer-pulse, win-glow — transform/opacity only)
  hooks/       useTimebank, useTableKeys (F/C/R + number presets), useNarration
  lib/         cards (parse, four-color), format (chips/BB tabular), sha (verify)
  components/
    table/     Table, Seat, Board, Pot, Card, TimerRing, ActionBar,
               BetSlider, ReconnectBanner, layout.ts (one table, two projections)
    lobby/     Lobby, FriendsPanel, CreateRoom
    auth/ landing/ settings/ replay/ fairness/  ui/ (shared kit)
```

## Protocol

`src/net/protocol.ts` mirrors `server/internal/protocol/messages.go`. The
shapes fully defined there (`Envelope`, `PlaceBet`, `HandDealt`, `FairReveal`,
`ErrorEvent`) match exactly; the remaining server events follow the shapes in
`client/PROMPT.md`. When integrating, regenerate/adjust these TS types against
the Go source as it grows. Strict TypeScript; no `any` in the protocol layer.

Game outcomes are never computed on the client — it renders server-confirmed
state and optimistically previews only the local player's own action.

## Provably-fair verification

The Fairness screen recomputes `SHA-256(seed)` in the browser
(`src/lib/sha.ts`) and checks it against the published commitment. The demo
hand ships a matching seed/commitment fixture so the flow is verifiable
offline.
