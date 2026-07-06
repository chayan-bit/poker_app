// Keeps the screen awake while a nearby session is active (issue #28 hardening).
// Phones auto-lock within a minute or two, which suspends timers and drops the
// peer out of the mesh mid-hand. We hold a navigator.wakeLock screen lock for the
// session, re-acquire it whenever the tab becomes visible again (the OS releases
// the sentinel on backgrounding), and release it on teardown. Feature-detected:
// browsers without the Wake Lock API (or that deny it) simply carry on.

import { useEffect } from "react";

interface WakeLockNavigator {
  wakeLock?: { request(type: "screen"): Promise<WakeLockSentinel> };
}

export function useWakeLock(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    const nav = navigator as Navigator & WakeLockNavigator;
    if (!nav.wakeLock) return;

    let sentinel: WakeLockSentinel | null = null;
    let released = false;

    const acquire = async (): Promise<void> => {
      try {
        sentinel = await nav.wakeLock!.request("screen");
      } catch {
        // Denied or unavailable in this context; degrade gracefully.
      }
    };
    const onVisible = (): void => {
      if (document.visibilityState === "visible" && !released) void acquire();
    };

    void acquire();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      released = true;
      document.removeEventListener("visibilitychange", onVisible);
      void sentinel?.release().catch(() => {});
    };
  }, [active]);
}
