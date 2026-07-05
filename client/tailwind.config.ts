import type { Config } from "tailwindcss";

// Tokens are the source of truth in src/index.css (CSS variables); Tailwind
// utilities read them via var() so themes swap by toggling a class on <html>.
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        surface: "var(--surface)",
        "surface-2": "var(--surface-2)",
        "surface-3": "var(--surface-3)",
        felt: "var(--felt)",
        "felt-edge": "var(--felt-edge)",
        ink: "var(--ink)",
        "ink-dim": "var(--ink-dim)",
        "ink-faint": "var(--ink-faint)",
        line: "var(--line)",
        gold: "var(--gold)",
        "action-blue": "var(--action-blue)",
        danger: "var(--danger)",
        "card-face": "var(--card-face)",
      },
      fontFamily: {
        sans: ["Inter", "ui-sans-serif", "system-ui", "sans-serif"],
      },
      transitionTimingFunction: {
        "ease-out-cubic": "cubic-bezier(0.22, 1, 0.36, 1)",
      },
      fontVariantNumeric: {
        tabular: "tabular-nums",
      },
    },
  },
  plugins: [],
} satisfies Config;
