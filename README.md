# poker_app

A fast, fair, social poker platform.
Playable instantly on the web, installable on Android and iPhone, built around one promise: **zero perceived lag**.

## Why build another poker app

Market research on the incumbents (July 2026) shows a consistent set of failures:

| App | What it does well | Where it fails |
|-----|-------------------|----------------|
| Zynga Poker | Huge casual player base | Crashes, glitches, ad spam, aggressive chip-buying pressure, "rigged deal" perception |
| WSOP | Brand, tournaments | Endless popups begging for money, UI downgrade drove player exodus, "algorithm makes you lose so you buy chips" complaints |
| PokerStars Play | Silky fast mobile-first UX, 14+ variants | Weak private/home-game social layer in the free app |
| ClubGG / PokerBros / PPPoker | Private clubs, play with friends, variants | Mobile-only or weak web clients, club onboarding friction, agent/union sketchiness |
| Poker Now (browser) | Zero-friction link-and-play, no download | 10-player cap, poor mobile experience, no accounts/persistence, no polish |

The wedge nobody owns: **PokerStars-grade snappiness + Poker Now-grade frictionless friend games + ClubGG-grade club features, on web AND native mobile, with no ads and no dark patterns.**

## Core features

- **Account system** with guest-to-account upgrade (start playing from a link, claim your chips later).
- **Fake money economy**: daily chip refills, no real-money purchases, no ads, no "buy chips" popups ever.
- **Public lobbies**: cash tables and sit-and-gos across stakes, quick-seat in one tap.
- **Private lobbies**: invite-link home games with custom rules (blinds, timers, bomb pots, run-it-twice, straddles).
- **Friends layer**: friend list, presence, "rail me" spectating, recurring scheduled home games.
- **Cross-platform**: one codebase, playable in any browser, installable from the App Store and Play Store.

## The differentiators

1. **Snappy by design**: 100 ms input-feedback budget, optimistic UI with server reconciliation, 60 fps transform-only animations. Full budget in [docs/Design_suite.md](docs/Design_suite.md).
2. **Provably fair shuffle**: commit-reveal deck seeds so any player can cryptographically verify no deal was rigged. This directly attacks the #1 trust complaint against Zynga and WSOP.
3. **Hand replayer with equity graph**: every hand is replayable with street-by-street equities, shareable as a link.
4. **Session highlight reels**: auto-generated recap of your biggest pots and baddest beats after each session.
5. **Cross-device handoff**: fold on your phone on the couch, pick up the same seat on your laptop.

## Tech direction (summary)

- **Client**: React + TypeScript + Vite, table rendered with GPU-friendly transforms, wrapped with Capacitor for iOS/Android.
- **Server**: FastAPI + WebSockets, authoritative game engine, PostgreSQL for persistence, Redis for table state and pub/sub.
- **Realtime target**: sub-100 ms RTT via regional gateways, protocol messages under 1 KB.

Rationale, alternatives considered, and the full design system live in [docs/Design_suite.md](docs/Design_suite.md).

## Repository layout

```
poker_app/
├── README.md            # you are here
├── CLAUDE.md            # agent/contributor working instructions
└── docs/
    └── Design_suite.md  # design decisions: UX, visual system, motion, architecture, fun features
```

## Status

Pre-implementation: research and design phase complete, scaffolding next.

## Research sources

- [Zynga Poker problems (justuseapp)](https://justuseapp.com/en/app/354902315/zynga-poker-texas-holdem/problems), [Zynga Trustpilot](https://www.trustpilot.com/review/www.zynga.com)
- [WSOP Trustpilot reviews](https://www.trustpilot.com/review/playwsop.com)
- [PokerNews best poker apps 2026](https://www.pokernews.com/best-poker-apps.htm)
- [ClubGG vs PokerBros 2026](https://bluffingmonkeys.com/clubgg-vs-pokerbros-2026-best-poker-app/), [PPPoker vs PokerBros vs ClubGG](https://apppoker.deals/articles/pppoker-pokerbros-clubgg-app-comparison)
- [Poker Now review (Worldpokerdeals)](https://worldpokerdeals.com/rakeback-deals/pokernow-review)
- [Optimizing poker apps for low latency](https://innosoft-group.com/how-to-optimize-poker-app-performance-for-low-latency-gameplay/), [WebSockets in realtime gaming (Pusher)](https://pusher.com/blog/websockets-realtime-gaming-low-latency/)
