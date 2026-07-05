# Client-generation prompt for poker_app

Paste the block below into an AI design/app-builder tool (v0, Lovable, Bolt, Claude Artifacts, etc.).
It is self-contained: it restates every constraint the tool needs so it does not have to see the repo.
After generation, the output goes in `client/` and is wrapped with Capacitor for iOS/Android.

---

## PROMPT (copy from here)

Build the **client-side web app** for a cross-platform social poker game called **poker_app**. It must run in any browser and later be wrapped with Capacitor for iOS/Android from the same codebase. Do NOT build a backend - the server already exists (Go + WebSockets, authoritative). Build only the client: UI, state, animations, and the WebSocket client layer that talks to the server protocol below.

### Stack (use exactly this)
- React 19 + TypeScript + Vite.
- Tailwind CSS for styling, with design tokens defined as CSS variables (see palette).
- Framer Motion allowed ONLY for non-table chrome (menus, modals, lobby transitions). The poker table's own animations (dealing, chip movement, pot push, card flip) must be hand-rolled CSS transforms / Web Animations API - NOT Framer Motion, NOT JS requestAnimationFrame loops - so the main thread stays free for the WebSocket.
- State: lightweight store (Zustand or equivalent). No Redux.
- Target modern evergreen browsers; mobile Safari and Chrome are first-class.

### The one hard rule: it must feel instant (no lag)
This is the product's entire identity. Enforce these as non-negotiable:
1. **Input feedback < 100 ms, always local.** Tapping fold/check/call/bet/raise updates the UI immediately (optimistic), tagged as pending, before the server confirms. Never show a spinner in the action path.
2. **Optimistic UI with reconciliation.** The local player's own action renders instantly and is confirmed or rolled back by the server. On the rare rollback, do a subtle shake on the action bar - never a modal. Opponents' actions render only when a server event arrives (no client-side game logic).
3. **Animations are transform/opacity only** (GPU-composited), 150-250 ms, single ease-out-cubic easing. The pot-push to a winner may take up to 400 ms (the emotional payoff). Nothing else on the table animates during a hand.
4. **The table is a stable scene graph.** Seats, board, and pot are persistent DOM nodes that MOVE via transforms; do not remount them per event, and do not re-render the whole table on every state change - re-render per discrete event, never per frame.
5. Respect `prefers-reduced-motion`: collapse all animation to 80 ms fades. Provide an in-app "reduced motion" toggle too.
6. Code-split so the table view loads first; lazy-load lobby extras, settings, and the hand replayer.

### Screens to build
1. **Landing / instant join.** A private game is just a URL (`/t/:joinCode`). Opening it seats you as a guest after a single name prompt - nothing else. Also a simple home for logged-in users.
2. **Auth.** Email + OAuth login, plus "continue as guest". Guest state persists (device token) and can upgrade to a full account later without losing chips/history. (Client-side flows and screens only; call the server's auth endpoints.)
3. **Lobby.**
   - Public: list of cash tables and sit-and-gos by stake tier; the hero action is one-tap **Quick Seat**.
   - Private: "Create room" (host sets blinds, timer length, buy-in range, max seats up to 10, and rule toggles: run-it-twice, bomb-pot frequency, straddles, dealer's-choice rotation) and "Join by 6-char code".
   - Friends panel: friend list with presence (at a table / in lobby / offline), one-tap "join their table" and "rail them" (spectate).
4. **The poker table (the centerpiece).**
   - **Portrait mobile:** vertical table, your two hero cards bottom-center, action bar in the thumb-reachable bottom 25%.
   - **Landscape / desktop:** classic oval table. Implement ONE table with two projections of the same seat coordinates - do not build two separate tables.
   - Always visible without tapping: pot size, effective stacks, amount to call, each player's last action as a small persistent chip-tag ("raise 240") until the next street.
   - Bet controls: a horizontal slider PLUS preset pills (min / ⅓ / ½ / ⅔ / pot / all-in); presets configurable.
   - Timebank: a shrinking ring around the active player's avatar, shifting color only in the final 5 seconds. No full-screen flashing.
   - Optional toggle to show amounts in big blinds (BB) instead of chips.
   - Reconnect: on socket drop show a thin top banner and auto-resume; do not fold the player.
5. **Hand replayer** (can be a secondary bundle): scrub any completed hand street-by-street with an equity graph; shareable as a link that renders for non-users.
6. **Settings:** theme (dark default / light), four-color deck toggle, sound toggle, reduced-motion, bet-preset config, BB display toggle.
7. **Fairness screen:** plainly explains the provably-fair commit-reveal shuffle and lets a player paste a revealed seed + commitment to verify a past hand client-side (SHA-256 in the browser).

### Visual design system (follow precisely)
- Direction: "modern card room at night". Calm dark surfaces; the cards and chips are the loudest objects. No skeuomorphic wood/leather, no casino neon.
- Tokens (define as CSS variables, ship dark as the target + a light theme):
  - Surface charcoal `#101418`; table felt deep green `#0E3B2E` with a subtle radial vignette.
  - Chip gold `#E8B44C` for pots/wins; action blue `#4C9AE8` for the active player; danger red `#E85C5C` reserved for fold/timeout only.
  - Cards: white faces, oversized rank indices, four-color deck as a toggle; legibility over beauty.
  - Type: one variable sans (Inter). **Tabular numerals everywhere numbers appear** so chip counts never change width as they tick.
- Accessibility: WCAG AA contrast in both themes; suits distinguishable without color (four-color deck helps); full keyboard play on web (F = fold, C = check/call, R = raise, number keys size the bet); minimum 44px touch targets; action buttons are the largest elements on the mobile screen. Drive screen-reader narration from the same event stream the animations consume so audio can't drift from the visuals.

### Motion vocabulary (only these animate during a hand)
`deal`, `flip`, `chip-bet`, `pot-merge`, `pot-push`, `fold-muck`, `timer-pulse`, `win-glow`. Implement each as a small reusable transform-based primitive.

### WebSocket protocol (match exactly - the server owns these shapes)
- One persistent WebSocket. Every message is an envelope: `{ v: number, type: string, seq?: number, data?: object }`. `v` is the protocol version (start at 1). `seq` is a monotonic sequence number on server events; if you receive a gap, send one `resync` command and apply the returned snapshot (do not tear down the UI).
- **Client -> server commands** (imperative): `join_table`, `sit_down`, `place_bet` (payload `{ tableId, kind: "check"|"call"|"bet"|"raise"|"fold", amount }` where `amount` is the target to-amount for bet/raise), `leave_table`, `resync`.
- **Server -> client events** (past tense, render-only): `hand_dealt` (`{ tableId, handId, commitment, yourSeat, yourHole: string[], buttonSeat, blinds:[sb,bb] }` - only YOUR hole cards are ever sent), `bet_placed`, `street_advanced`, `showdown`, `table_snapshot`, `seat_update`, `fair_reveal` (`{ handId, commitment, seed }` - after the hand, for verification), `error` (`{ code, message }`).
- Cards are strings like `"As"`, `"Td"`, `"2c"` (rank char + suit char c/d/h/s).
- Never compute game outcomes on the client. Render server-confirmed state; optimistically preview only the local player's own action.

### Deliverables
- A complete, runnable Vite React+TS project: `src/` with components, the Zustand store, a typed WebSocket client module (`src/net/`) with the envelope/command/event types above, Tailwind config with the tokens, and a mock server / fixture mode so the table is fully demoable without the real backend.
- Clean component structure: `Table`, `Seat`, `Board`, `Pot`, `ActionBar`, `BetSlider`, `Lobby`, `FriendsPanel`, `HandReplayer`, `FairnessVerifier`, plus the motion primitives.
- Keep files small and focused (~200-400 lines each). Strict TypeScript, no `any` in the protocol layer.

Prioritize the poker table screen and its snappiness above everything else; make that one screen feel flawless first.

## PROMPT (end)

---

### After the tool generates the client

1. Drop the generated project into `client/`.
2. Wire the WebSocket client's types against `server/internal/protocol/messages.go` (that Go file is the source of truth; regenerate/adjust TS types to match exactly).
3. Add Capacitor: `npm i @capacitor/core @capacitor/cli && npx cap init` then `npx cap add ios && npx cap add android`, pointing `webDir` at the Vite `dist/`.
4. Run against `pokerd` locally (`make run`, server on `:8080`, connect the client to `ws://localhost:8080/ws`).
