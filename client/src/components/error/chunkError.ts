// Detects a dynamic-import (lazy chunk) load failure. Kept in its own module so
// both ErrorBoundary.tsx (component file) and lazyWithReload.ts can import it
// without tripping react-refresh's "components-only export" rule.

/** A dynamic-import failure surfaces as a ChunkLoadError or a message about a
 *  failed module fetch (common after a redeploy invalidates hashed filenames). */
export function isChunkLoadError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  const name = error.name || "";
  const message = error.message || "";
  return (
    name === "ChunkLoadError" ||
    /Loading chunk|dynamically imported module|Importing a module script failed|Failed to fetch dynamically/i.test(
      message,
    )
  );
}
