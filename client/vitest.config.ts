// Vitest configuration. Node environment by default: the high-value modules
// under test (mappers, coordinator math, fair-seed crypto, mesh view, store
// dispatch, API error paths) are pure or WebCrypto-only, and Node 20+ ships
// WebCrypto globally. Tests that need window/localStorage opt into jsdom with
// a "// @vitest-environment jsdom" docblock at the top of the file.

import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    // Mirrors vite.config.ts / tsconfig.app.json ("@/*" -> "./src/*").
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
    coverage: {
      provider: "v8",
      // Scoped to the modules the unit suite targets; React components and
      // transport glue are exercised by E2E, not chased for line coverage.
      include: [
        "src/components/history/mapRecord.ts",
        "src/local/coordinator.ts",
        "src/local/fairmp.ts",
        "src/local/meshview.ts",
        "src/local/wire.ts",
        "src/net/api.ts",
        "src/net/mode.ts",
        "src/store/gameStore.ts",
      ],
      reporter: ["text", "html"],
    },
  },
});
