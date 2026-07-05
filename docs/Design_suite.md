# Design Suite - poker_app

Design decisions for poker_app: product positioning, UX, visual system, motion, architecture-for-snappiness, and experimental features.
This is the source of truth for "why it looks and feels the way it does".

## 1. Positioning and research findings

Research across app-store reviews, Trustpilot, and poker-app comparison sites (July 2026) surfaced five recurring failures in existing apps.

1. **Lag and jank**: Zynga and WSOP reviews are full of crash, stutter, and reconnect complaints; heavy DOM/animation work and chatty networking make tables feel mushy.
2. **Dark patterns**: constant chip-purchase popups, ads between hands, and "losing streaks that push you to buy" dominate negative reviews.
3. **Rigged-deal distrust**: the single most common substantive complaint is "the deals aren't random"; no mainstream free app offers verifiable fairness.
4. **Friend-game friction**: club apps (ClubGG, PokerBros, PPPoker) require club IDs, agent onboarding, and approval flows just to play with friends; Poker Now nails link-and-play but caps at 10 players, has no accounts, and is weak on mobile.
5. **Platform gaps**: club apps are mobile-only, Poker Now is web-only; nobody serves web + iOS + Android with one consistent, polished experience.

Positioning: *the fast, fair, friendly poker app* - PokerStars-grade feel, Poker Now-grade friction, zero monetization pressure.

## 2. Product pillars

- **Fast**: every interaction acknowledged within 100 ms; the table never blocks on the network.
- **Fair**: provably fair shuffle, transparent RNG, public hand histories.
- **Friendly**: playing with friends is a first-class flow, not a club bureaucracy.
- **Free of manipulation**: fake-money economy with daily refills; chips are a scoreboard, not a revenue lever.

## 3. Platform and stack decisions

**Decision: web-first React + TypeScript + Vite client, wrapped with Capacitor for the app stores; Go + WebSockets authoritative server (a single lightweight static binary).**

Server-language note: the original sketch used Python/FastAPI, but the server sits directly in the latency hot path, so it was switched to Go. Go gives compiled per-message handling with no interpreter/GIL overhead, goroutine-per-connection concurrency (each table an isolated single-writer goroutine, so the pure engine needs no locks), sub-millisecond GC for flat tail latency, and one static binary that deploys cleanly to regional edge gateways for the sub-100 ms RTT target. The pure poker engine lives in its own `engine` package with no I/O so it stays exhaustively testable.

Alternatives considered:

| Option | Verdict |
|--------|---------|
| Flutter | Excellent canvas perf, but web output is heavy and worse for instant link-joining, which is our killer friction-reducer. |
| React Native + Expo | Good native feel, but react-native-web tables are second-class and we'd fight the layout system for the table view. |
| Unity/game engine | Overkill; huge bundles kill instant web join and app size. |
| **Web-first + Capacitor** | One codebase, instant browser join via link (the Poker Now trick), real store presence, and the team's existing React/FastAPI expertise. |

Consequence: we must earn native-feeling performance in the browser, which is why the motion and rendering budgets below are strict.

## 4. The snappiness contract (anti-lag architecture)

This is the section that exists because "most poker apps are laggy".

### 4.1 Perceived-latency budget

- Touch/click to visual feedback: **< 100 ms**, always local, never waits on network.
- Action to server-confirmed state render: **< 150 ms perceived** (optimistic UI bridges real RTT).
- Card deal / pot push animations: **150-250 ms**, never longer; animations are feedback, not theater.
- Cold load to seated at a table (web): **< 3 s** on 4G; code-split so the table bundle loads first and lobby extras lazy-load.

### 4.2 Optimistic UI with server reconciliation

The client renders the local player's own action (bet slider commit, fold, check) instantly and tags it pending.
The authoritative server confirms or rejects; on rejection (rare: race with timer expiry) the client rolls back with a subtle shake, never a modal.
Opponents' actions render only on server events, so no ghost states are possible.

### 4.3 Rendering rules

- Table animations use **transform and opacity only** (GPU-composited); no layout-affecting properties mid-hand.
- The table is a fixed scene graph: seats, board, pot are stable DOM nodes that move via transforms; React re-renders happen per-event, not per-frame.
- Chip and card motion via WAAPI/CSS transitions, not JS rAF loops, so the main thread stays free for WebSocket handling.
- Everything on the table is vector or pre-rasterized sprites; no runtime image decoding mid-hand.

### 4.4 Network rules

- One persistent WebSocket per client; binary-lean JSON messages under 1 KB; no polling anywhere.
- Regional gateway servers (start single-region, design for multi-region) targeting sub-100 ms RTT for the core audience.
- Delta state updates, not full table snapshots, with a versioned sequence number; a gap triggers a single snapshot resync.
- Reconnect is a first-class flow: socket drop shows a thin banner, auto-resumes within one hand, and the server sits you out rather than folding you for 10 s.

## 5. Visual design system

### 5.1 Direction

"Modern card room at night": dark-first UI, deep felt greens/charcoals, warm chip accents, high-contrast cards.
No skeuomorphic wood-and-leather kitsch (Zynga) and no casino-neon assault (WSOP); calm surfaces that make the cards and chips the loudest objects on screen.

### 5.2 Tokens

- **Surfaces**: near-black charcoal `#101418`, table felt deep green `#0E3B2E` with a subtle radial vignette.
- **Accents**: chip gold `#E8B44C` for pots/wins, action blue `#4C9AE8` for the active player, danger red `#E85C5C` reserved for fold/timeout only.
- **Cards**: oversized indices, four-color deck as a default-off toggle, white faces at maximum contrast; card legibility beats card beauty.
- **Type**: one variable sans (Inter or similar), tabular numerals everywhere numbers appear; chip counts must not jitter in width as they tick.
- Light theme exists but dark is the design target; both ship from day one via tokens.

### 5.3 Layout

- **Portrait mobile**: vertical table (ClubGG-style), hero cards bottom-center, action bar thumb-reachable in the bottom 25% of the screen.
- **Landscape web/tablet**: classic oval table; the same scene graph re-projects seat coordinates, so there is one table implementation with two projections.
- Bet sizing: horizontal slider plus preset pills (min / ⅓ / ½ / ⅔ / pot / all-in); presets are configurable per player because serious players live on presets.
- Everything reachable one-handed on mobile; no action ever hides behind a scroll.

### 5.4 Information design

- Pot, effective stacks, and to-call amount always visible without taps.
- Bet amounts render in big-blind units as an optional toggle (players like Chayan think in BBs).
- Timebank shown as a shrinking ring around the avatar, color-shifting only in the final 5 s; no full-screen flashing.
- Last action per seat persists as a small chip-tag ("raise 240") until the next street, so returning glances re-orient instantly.

## 6. Core UX flows

### 6.1 Join friction (the Poker Now lesson)

- A private game is a URL; clicking it on any device seats you as a guest with a name prompt, nothing else.
- Guest accounts silently persist (device token) and can be upgraded to a full account later, keeping chips and history.
- Full accounts: email/OAuth, needed only for cross-device identity, friends, and the economy.

### 6.2 Lobbies

- **Public**: quick-seat by stake tier; one tap from home screen to dealt-in; filters for variant/table size but quick-seat is the hero path.
- **Private**: host creates a room, gets a link + 6-char code; host controls blinds, timer, buy-in range, variants, bomb-pot frequency, run-it-twice, straddles, and who may join (link-anyone, friends-only, approval).
- Private rooms support up to 10 seats per table and multi-table rooms, breaking the Poker Now 10-player ceiling for bigger home games.

### 6.3 Friends layer

- Friend list with presence ("at a table", "in lobby", "offline") and one-tap "join their table" / "rail them".
- Recurring home games: hosts schedule "every Friday 9 pm", invitees RSVP, the room auto-opens with pinned settings, and the app pushes a reminder.
- Spectating ("railing") shows the table without hole cards; hole cards optionally revealed to railbirds after the hand completes.

### 6.4 The economy (fake money, no manipulation)

- Daily refill, escalating streak bonus, and a bankruptcy floor so no one is ever locked out.
- Chips have no purchase path; leaderboards and cosmetic table themes are the aspiration layer.
- All-time and per-session stats (VPIP, PFR, won/lost by position) are free, because incumbents paywall basic stats.

## 7. Fairness as a feature

- **Commit-reveal shuffle**: before each hand the server publishes `SHA-256(seed)`; after the hand it reveals the seed, and any client (or the open-source verifier CLI) can recompute the shuffle and confirm the deal.
- RNG: CSPRNG seeded per hand; the seed commit is stored in the permanent hand history.
- Hand histories are exportable (standard text format) and shareable as replay links.
- A "Fairness" screen in-app explains the scheme in plain language; trust is a UX deliverable, not just a protocol property.

## 8. Motion design language

- Motion vocabulary of ~8 named animations (deal, flip, chip-bet, pot-merge, pot-push, fold-muck, timer-pulse, win-glow); nothing else animates during a hand.
- All motion 150-250 ms with a single standard easing (ease-out-cubic); winners' pot-push may take 400 ms because it is the emotional payoff.
- Haptics on native (Capacitor): light tick on your turn, medium on pot win; silent on web.
- A "reduced motion" setting collapses all animation to 80 ms fades and is also auto-enabled by OS-level prefers-reduced-motion.
- Sound: soft felt/chip foley, off by default on web, on by default on native, per-event granularity in settings.

## 9. Ingenious extras (the fun shelf)

Feature-flagged experiments layered on top of the lean core, roughly ordered by value/effort.

1. **Equity-graph hand replayer**: scrub any completed hand street by street with live equity percentages; share as a link that renders in the browser for non-users (an acquisition loop).
2. **Session highlight reel**: auto-generated 30-second recap of your session's biggest pot, best bluff (won without showdown vs. equity), and worst beat; one tap to share.
3. **Cross-device handoff**: your seat follows your account; open the web app mid-hand and tap "continue here", and the phone client gracefully becomes a spectator.
4. **Table time capsule**: private rooms accumulate a persistent "wall" of memorable hands and standings across weeks of a recurring home game, turning a game night into a season.
5. **Ghost of you**: after a session, replay key hands against a simple bot playing *your own* historical tendencies, to see your leaks from the other side of the table.
6. **Bomb pots, run-it-twice, 7-2 bounty**: house-rule toggles that club apps have proven popular, exposed as simple switches for private hosts.
7. **Throwables with a budget**: lightweight emote throws (capped per hand, rendered as pure transform sprites) for table banter without Zynga-style spam or perf cost.
8. **Dealer's choice rotation**: private rooms can rotate variant per orbit (NLHE, PLO, Short Deck) like a real home game.

## 10. Accessibility

- WCAG AA contrast on all text and on card suits in both themes; four-color deck helps color-blind players and ships day one.
- Full keyboard play on web (F/C/R/number keys for bet sizing) which doubles as the power-user speed layer.
- Screen-reader narration of game events from the same event stream the animations consume, so it can never drift from the visual truth.
- Minimum 44 px touch targets; action buttons are the largest elements on the mobile screen.

## 11. Open questions

- Multi-tabling on web: valuable for serious players, but it multiplies rendering budget; decide after the single-table experience hits budget.
- Voice/video at private tables (Poker Now offers webcam): high social value, high complexity; likely a WebRTC v2 feature.
- Anti-collusion for public lobbies (fake money still attracts chip-dumpers): server-side pattern flags, deferred until public traffic exists.
