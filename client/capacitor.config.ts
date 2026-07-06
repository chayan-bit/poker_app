import type { CapacitorConfig } from "@capacitor/cli";

// After `npm run build`, run `npx cap add ios` / `npx cap add android`, then
// `npx cap sync`. webDir points at Vite's dist output.
//
// NOTE: `appId` is a PLACEHOLDER. Change `com.felt.poker` to the real reverse-DNS
// bundle id you own before store submission (Apple App ID / Android applicationId).
// Changing it later means regenerating the ios/ and android/ native projects.
const config: CapacitorConfig = {
  appId: "com.felt.poker",
  appName: "Felt",
  webDir: "dist",
  server: {
    androidScheme: "https",
  },
  plugins: {
    // Splash background matches the app surface token (--surface #0c1013).
    // Trivial, config-only; no src changes required.
    SplashScreen: {
      backgroundColor: "#0B0F14",
      showSpinner: false,
      androidScaleType: "CENTER_CROP",
      splashFullScreen: true,
      splashImmersive: true,
    },
  },
};

export default config;
