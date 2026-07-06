# Mobile (Capacitor iOS + Android)

This wraps the same Vite build (`dist/`) as native iOS and Android apps via Capacitor 6.
The web client is authoritative for all UI; the native shells only host the WebView.

## App identity

The `appId` in `capacitor.config.ts` is `com.felt.poker`, a PLACEHOLDER.
Change it to the reverse-DNS bundle id you actually own before any store upload.
Changing `appId` after the fact means regenerating `ios/` and `android/`.
The `appName` is `Felt`.

## How the packaged app finds the server

The client reads two build-time env vars (Vite inlines them at build):
`VITE_API_URL` is the HTTP base for REST (auth, lobby, hand history).
`VITE_WS_URL` is the WebSocket URL for live table state.
If unset, the client falls back to `http://localhost:8080` (see `src/net/api.ts`).

A packaged app cannot reach your dev machine's `localhost`, so you MUST set both
vars to a publicly reachable HTTPS/WSS origin at build time, then rebuild and sync:

```sh
VITE_API_URL="https://api.yourdomain.com" \
VITE_WS_URL="wss://api.yourdomain.com/ws" \
npm run build && npx cap sync
```

## Why cleartext localhost is dev-only

`server.androidScheme` is `https`, so the WebView origin is an HTTPS context.
Cleartext `http://`/`ws://` to `localhost` works only under the emulator/simulator
against a locally forwarded dev server; it is not shipped.
iOS ATS and Android network-security config both block arbitrary cleartext in release,
so production must use HTTPS/WSS endpoints, which is why the build-time vars above are required.

## Build + open the native IDEs

```sh
npm run build          # produce dist/
npx cap sync           # copy dist/ into ios/ and android/, refresh native deps
npx cap open ios       # opens ios/App/App.xcworkspace in Xcode
npx cap open android   # opens android/ in Android Studio
```

In Xcode: select a Simulator or device, then Product > Run.
In Android Studio: let Gradle sync, pick a device/emulator, then Run.

## Icons + splash

Generated from `resources/icon.png` and `resources/splash.png` (spade-in-ring
monogram, gold `#D4A64A` on `#0B0F14`) via `@capacitor/assets` into both native
projects. To regenerate, see `resources/README.md`.
The splash background is also set in `capacitor.config.ts` (`SplashScreen.backgroundColor`).

## Store-packaging checklist

Done:
- Capacitor iOS project under `ios/` (CocoaPods installed).
- Capacitor Android project under `android/`.
- App icons (all densities) and splash screens (light + dark) in both projects.
- Splash background color wired via `@capacitor/splash-screen` config.
- Viewport meta: `viewport-fit=cover`, `user-scalable=no`, theme-color `#0B0F14`.

Not done (requires a human with signing identities and store accounts):
- Set the real `appId` / bundle id and app display name for each store.
- iOS: Apple Developer account, App ID, provisioning profiles, signing team in Xcode,
  version/build numbers, App Store Connect listing, screenshots, privacy manifest.
- Android: `applicationId`, upload keystore + `signingConfig` (do NOT commit the keystore),
  `versionCode`/`versionName`, Play Console listing, screenshots, data-safety form.
- Push/status-bar theming beyond splash: a `@capacitor/status-bar` runtime call
  (set style/overlay) would live in `src/main.tsx`; deferred because `src/` is owned
  elsewhere. Add it when integrating native lifecycle.
- Deep links / universal links if server auth flows need them.
- Real device QA of safe-area insets on notched hardware.
