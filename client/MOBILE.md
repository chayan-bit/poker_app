# Mobile (Capacitor iOS + Android)

This wraps the same Vite build (`dist/`) as native iOS and Android apps via Capacitor 6.
The web client is authoritative for all UI; the native shells only host the WebView.

## App identity

The `appId` in `capacitor.config.ts` is `com.felt.poker`, a PLACEHOLDER.
CONFIRM this value before store submission: change it to the reverse-DNS bundle id
you actually own (Apple App ID / Android `applicationId`).
Changing `appId` after the fact means regenerating `ios/` and `android/`.
The `appName` is `Felt`; `webDir` is `dist` (Vite output).
Android `applicationId`/`namespace` is `com.felt.poker` (in `android/app/build.gradle`);
iOS bundle id is set in Xcode signing. Keep all three in sync with `appId`.

## Platform targets (store minimums)

- Android: `compileSdkVersion`/`targetSdkVersion` are `35` (Google Play minimum),
  `minSdkVersion` `22`, in `android/variables.gradle`. `versionCode 1` /
  `versionName "1.0.0"` in `android/app/build.gradle` - bump per release.
- iOS: deployment target and version/build come from the Xcode project + signing.
- Capacitor 6 (`@capacitor/*` v6). Native plugins used at runtime:
  `@capacitor/app` (back button + deep links), `@capacitor/status-bar`,
  `@capacitor/keyboard`, `@capacitor/splash-screen`.

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

If `VITE_API_URL` is unset in a NATIVE build, the app no longer silently falls
back to `http://localhost:8080` (which would ship a dead store binary). Instead
`src/net/api.ts` throws a clear `misconfigured_api_url` error on the first REST
call, surfaced to the user as a "no server configured - please refresh" banner.
The localhost default is retained for web dev only. So: a release build with no
`VITE_API_URL` fails loudly and visibly rather than appearing to hang.

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

### Headless release builds (CI / no IDE)

Android, with a reproducible Nix-pinned SDK (platforms 34+35, build-tools 35,
JDK 17 - Gradle 8.2.1 does not run on Java 21):

```sh
nix-shell client/android/shell.nix --run 'cd client/android && ./gradlew :app:assembleRelease'
# -> client/android/app/build/outputs/apk/release/app-release-unsigned.apk
```

The APK is unsigned. Sign it for the Play Store with your upload key
(`apksigner sign --ks <keystore> ...`) or configure a `signingConfig` in
`android/app/build.gradle` and build an AAB with `:app:bundleRelease`.

iOS release/archive requires the Xcode iOS platform component installed
(`xcodebuild -downloadPlatform iOS` if missing) plus an Apple Developer signing
identity + provisioning profile:

```sh
xcodebuild -workspace ios/App/App.xcworkspace -scheme App -configuration Release \
  -archivePath build/App.xcarchive archive
xcodebuild -exportArchive -archivePath build/App.xcarchive \
  -exportOptionsPlist ExportOptions.plist -exportPath build/ipa
```

Set the Team + bundle id (`com.felt.poker` is a placeholder - confirm before
submission) in Xcode's Signing & Capabilities first.

## Icons + splash

Generated from `resources/icon.png` and `resources/splash.png` (spade-in-ring
monogram, gold `#D4A64A` on `#0B0F14`) via `@capacitor/assets` into both native
projects. To regenerate, see `resources/README.md`.
The splash background is also set in `capacitor.config.ts` (`SplashScreen.backgroundColor`).

## Native lifecycle integrations (wired in `src/App.tsx`)

All guarded by `Capacitor.isNativePlatform()`, so web is unaffected:

- Status bar: `StatusBar.setStyle({ style: Style.Dark })` (light content over the
  near-black `#0B0F14` surface) on both platforms; Android also sets the bar
  background to `#0B0F14`.
- Keyboard: `Keyboard.setResizeMode({ mode: Native })` at startup, plus
  `plugins.Keyboard.resize: "native"` in `capacitor.config.ts`, so the soft
  keyboard resizes the WebView and never covers auth/table inputs.
- Android back button: integrated via `App.addListener('backButton', ...)`.
  Back navigates within the router; leaving a LIVE table prompts a confirm and
  releases the seat (`leave_table` then disconnect); at the root the app exits.
  Without this, hardware/gesture back would pop a live table with no warning.

## Permissions and usage strings

iOS (`ios/App/App/Info.plist`):
- `NSLocalNetworkUsageDescription` - required for nearby LAN-only WebRTC; without
  it mDNS/local peers never resolve and the data channel silently fails.
- `NSBonjourServices` - `_felt._tcp` / `_felt._udp` PLACEHOLDER service types.
  Confirm the exact type the nearby module registers before submission.
- `NSCameraUsageDescription` - QR-code scanning to join a nearby table.
- `CFBundleURLTypes` - custom scheme `felt://` for deep links.

Android (`android/app/src/main/AndroidManifest.xml`):
- `INTERNET`, `ACCESS_NETWORK_STATE`.
- `ACCESS_WIFI_STATE`, `CHANGE_WIFI_MULTICAST_STATE`, `NEARBY_WIFI_DEVICES`
  (`neverForLocation`) - nearby LAN WebRTC + mDNS discovery.
- `CAMERA` (with `uses-feature ... required="false"`) - QR scanning.

## Deep links (`/t/:joinCode`)

An incoming join URL routes into the join flow via `App.addListener('appUrlOpen')`
in `src/App.tsx` (`joinPathFromUrl` extracts `/t/<code>`).

- Custom scheme `felt://t/<code>` works out of the box: registered in `Info.plist`
  (`CFBundleURLTypes`) and `AndroidManifest.xml` (VIEW intent-filter).
- Verified https links (Universal Links / App Links) need a domain you control:
  - Android: replace the PLACEHOLDER host `links.felt.app` in the `autoVerify`
    intent-filter and serve `/.well-known/assetlinks.json`.
  - iOS: add an Associated Domains entitlement (`applinks:links.felt.app`) in
    Xcode and serve `/.well-known/apple-app-site-association`. PLACEHOLDER domain.

## Store-packaging checklist

Done:
- Capacitor 6 iOS project under `ios/` (CocoaPods installed) and Android under `android/`.
- Android `targetSdk`/`compileSdk` bumped to 35 (Play minimum).
- App icons (all densities) and splash screens (light + dark) in both projects.
- Splash background color wired via `@capacitor/splash-screen` config.
- Viewport meta: `viewport-fit=cover`, `user-scalable=no`, theme-color `#0B0F14`.
- Native status-bar/keyboard/back-button/deep-link wiring (see above).
- Permission usage strings for nearby (local network + camera).

Not done (requires a human with signing identities and store accounts):
- Confirm/replace the real `appId` / bundle id and display name for each store.
- Replace the PLACEHOLDER deep-link domains and Bonjour service types, and publish
  the App Links / Universal Links association files.
- iOS: Apple Developer account, App ID, provisioning profiles, signing team in Xcode,
  version/build numbers, Associated Domains entitlement, App Store Connect listing,
  screenshots, privacy manifest.
- Android: upload keystore + `signingConfig` (do NOT commit the keystore),
  per-release `versionCode`/`versionName`, Play Console listing, screenshots,
  data-safety form.
- Real device QA of safe-area insets on notched hardware (landscape included).
