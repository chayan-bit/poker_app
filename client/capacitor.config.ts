import type { CapacitorConfig } from "@capacitor/cli";

// After `npm run build`, run `npx cap add ios` / `npx cap add android`, then
// `npx cap sync`. webDir points at Vite's dist output.
const config: CapacitorConfig = {
  appId: "com.pokerapp.client",
  appName: "poker_app",
  webDir: "dist",
  server: {
    androidScheme: "https",
  },
};

export default config;
