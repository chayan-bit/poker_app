// Lazy-import wrapper that survives a stale deploy. When a dynamic chunk fails
// to load (old hashed filename no longer on the CDN after a redeploy), a bare
// `lazy(() => import(...))` throws with nothing to catch and white-screens the
// app. This helper retries the import once, and if it still fails it forces a
// single full reload (guarded against a reload loop) to pull the fresh manifest.
// If even the post-reload load fails, it rethrows so the surrounding
// ErrorBoundary renders a recoverable "reload" fallback instead of a blank page.

import { lazy, type ComponentType } from "react";
import { isChunkLoadError } from "./chunkError";

type Importer<T> = () => Promise<{ default: ComponentType<T> }>;

const RELOAD_FLAG_PREFIX = "felt.chunkReload.";

function hasReloadedFor(key: string): boolean {
  try {
    return window.sessionStorage.getItem(RELOAD_FLAG_PREFIX + key) === "1";
  } catch {
    return false;
  }
}

function markReloadedFor(key: string): void {
  try {
    window.sessionStorage.setItem(RELOAD_FLAG_PREFIX + key, "1");
  } catch {
    // Storage blocked: fall through; worst case we do not auto-reload.
  }
}

function clearReloadedFor(key: string): void {
  try {
    window.sessionStorage.removeItem(RELOAD_FLAG_PREFIX + key);
  } catch {
    // no-op
  }
}

/** Drop-in replacement for React.lazy with stale-deploy recovery. `key` must be
 *  stable and unique per route so the one-shot reload guard is per-chunk. */
export function lazyWithReload<T extends object = Record<string, never>>(
  key: string,
  importer: Importer<T>,
): ReturnType<typeof lazy<ComponentType<T>>> {
  return lazy(async () => {
    try {
      const mod = await importer();
      // A previously failed-then-recovered chunk: clear its guard so a future
      // stale deploy can auto-reload again.
      clearReloadedFor(key);
      return mod;
    } catch (error) {
      if (isChunkLoadError(error) && !hasReloadedFor(key)) {
        markReloadedFor(key);
        window.location.reload();
        // Return a never-resolving promise so React keeps the Suspense fallback
        // up while the reload navigates away.
        return new Promise<{ default: ComponentType<T> }>(() => {});
      }
      // Already reloaded once (or a non-chunk error): surface to the boundary.
      throw error;
    }
  });
}
