// Generates the brand source SVGs for Capacitor asset generation.
// Spade-in-a-ring monogram, gold on near-black, taken verbatim from
// src/components/ui/kit.tsx `SpadeMark` (viewBox 0 0 48 48).
// No heavy deps: writes SVG strings; rasterization is done by ImageMagick.
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const dir = dirname(fileURLToPath(import.meta.url));
const BG = "#0B0F14"; // app surface (matches --surface / splash background)
const GOLD = "#D4A64A"; // brand mark gold

const mark = (scale) => `
  <g transform="translate(${(48 - 48 * scale) / 2} ${(48 - 48 * scale) / 2}) scale(${scale})">
    <circle cx="24" cy="24" r="22.5" fill="none" stroke="${GOLD}" stroke-width="1.5" opacity="0.55"/>
    <path d="M24 10c5.2 5 11 9.6 11 15.2 0 3.4-2.4 5.6-5.3 5.6-1.7 0-3.2-.8-4.2-2.1.4 2.6 1.5 5 3.5 6.6h-10c2-1.6 3.1-4 3.5-6.6-1 1.3-2.5 2.1-4.2 2.1-2.9 0-5.3-2.2-5.3-5.6C13 19.6 18.8 15 24 10Z" fill="${GOLD}"/>
  </g>`;

// Icon: mark fills most of the 48-unit canvas on the app surface.
const icon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
  <rect width="48" height="48" fill="${BG}"/>${mark(0.82)}
</svg>`;

// Splash: same mark, smaller, centered on a large square canvas.
const splash = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
  <rect width="48" height="48" fill="${BG}"/>${mark(0.42)}
</svg>`;

writeFileSync(join(dir, "icon.svg"), icon.trim());
writeFileSync(join(dir, "splash.svg"), splash.trim());
console.log("wrote icon.svg + splash.svg");
