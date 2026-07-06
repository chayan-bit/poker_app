# Brand assets (Capacitor icon + splash source)

This folder holds the source art that `@capacitor/assets` rasterizes into the
per-platform icons and splash screens inside `ios/` and `android/`.

## Files

- `gen-source.mjs` - regenerates `icon.svg` and `splash.svg` from the `SpadeMark`
  path in `src/components/ui/kit.tsx` (spade in a ring, gold on near-black).
- `icon.svg` / `splash.svg` - vector sources.
- `icon.png` - 1024x1024, the master app icon (`@capacitor/assets` requires this name/size).
- `splash.png` / `splash-dark.png` - 2732x2732, the master splash (light and dark).

## Brand values

- Background / surface: `#0B0F14` (matches `--surface` in `src/index.css`).
- Mark gold: `#D4A64A`.

## Regenerate

```sh
# 1. Rebuild the SVG sources (only if the SpadeMark path changed):
node resources/gen-source.mjs

# 2. Rasterize the masters (ImageMagick; no new npm dep):
magick -background none -density 384 resources/icon.svg -resize 1024x1024 resources/icon.png
magick -background "#0B0F14" -density 96 resources/splash.svg -resize 2732x2732 resources/splash.png
cp resources/splash.png resources/splash-dark.png

# 3. Fan out into native projects (ephemeral, not a package.json dep):
npx --yes @capacitor/assets@3 generate \
  --iconBackgroundColor "#0B0F14" --iconBackgroundColorDark "#0B0F14" \
  --splashBackgroundColor "#0B0F14" --splashBackgroundColorDark "#0B0F14"
```

The generated `icons/` (PWA) and `public/manifest.webmanifest` outputs from
`@capacitor/assets` are intentionally deleted after generation; only the native
`ios/` and `android/` assets are kept for now.
