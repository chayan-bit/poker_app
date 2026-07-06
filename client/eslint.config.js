// ESLint flat config for the client. Recommended (not type-checked strict)
// presets: @eslint/js + typescript-eslint + react-hooks + react-refresh.
// `npm run lint` (eslint src) must exit 0 on the existing codebase; where a
// deliberate idiom conflicts with a rule, the rule is tuned here instead of
// editing source.

import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist", "ios", "android", "local-server-plugin", "coverage"] },
  {
    files: ["**/*.{ts,tsx,mts}"],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended,
      reactHooks.configs.flat["recommended-latest"],
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    linterOptions: {
      // core.ts carries `eslint-disable-next-line no-var` directives written
      // for a stricter ruleset that does not enable no-var here; reporting
      // them as unused would be permanent noise we cannot fix without
      // touching source.
      reportUnusedDisableDirectives: "off",
    },
    rules: {
      // Deliberate idiom across the table/tourney components: reset-on-prop-
      // change and the double-requestAnimationFrame entry-animation pattern
      // both call setState synchronously inside an effect on purpose (the
      // first offscreen frame must paint before animating in). Flagged by the
      // React-compiler-era preset; the pattern is intentional here.
      "react-hooks/set-state-in-effect": "off",
    },
  },
  {
    // Nearby.tsx forwards an imperative session handle kept in a ref into a
    // child once the phase flips; reading ref.current in render there is the
    // point of the design (the session is an external system, not view state).
    files: ["src/components/nearby/Nearby.tsx"],
    rules: {
      "react-hooks/refs": "off",
    },
  },
  {
    // These files deliberately co-locate small pure helpers (presenceLabel,
    // presetAmount, breakIntoChips) with the component that owns them; the
    // only cost is fast-refresh granularity in dev, not correctness.
    files: [
      "src/components/friends/PresenceDot.tsx",
      "src/components/table/BetSlider.tsx",
      "src/components/table/Chips.tsx",
    ],
    rules: {
      "react-refresh/only-export-components": "off",
    },
  },
  {
    // Node-side bridge/simulation harnesses for the WASM core: untyped JS
    // interop with wasm_exec.js makes `any` and mutable bindings part of the
    // territory. App code (.ts/.tsx) keeps the strict rules.
    files: ["src/local/*.mts"],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
      "prefer-const": "off",
    },
  },
);
