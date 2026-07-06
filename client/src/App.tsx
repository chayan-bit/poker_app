// App shell + routing. The table view is eagerly imported (priority bundle);
// everything else is lazy-loaded so the table renders first. Every lazy route
// goes through lazyWithReload so a stale-deploy chunk miss self-recovers rather
// than white-screening, and the whole tree sits under ErrorBoundary so no
// uncaught render error can blank the app inside the native shell.

import { Suspense, useEffect } from "react";
import {
  BrowserRouter,
  Routes,
  Route,
  useLocation,
  useNavigate,
  type NavigateFunction,
} from "react-router-dom";
import { MotionConfig } from "framer-motion";
import { Capacitor } from "@capacitor/core";
import { App as CapApp } from "@capacitor/app";
import { StatusBar, Style } from "@capacitor/status-bar";
import { Keyboard, KeyboardResize } from "@capacitor/keyboard";
import TablePage from "@/components/table/TablePage";
import Landing from "@/components/landing/Landing";
import { applyDocumentClasses, useSettings } from "@/store/settingsStore";
import { useGame } from "@/store/gameStore";
import { Cmd } from "@/net/protocol";
import { ErrorBoundary } from "@/components/error/ErrorBoundary";
import { lazyWithReload } from "@/components/error/lazyWithReload";

const Lobby = lazyWithReload("lobby", () => import("@/components/lobby/Lobby"));
const Auth = lazyWithReload("auth", () => import("@/components/auth/Auth"));
const Settings = lazyWithReload(
  "settings",
  () => import("@/components/settings/Settings"),
);
const HandReplayer = lazyWithReload(
  "replay",
  () => import("@/components/replay/HandReplayer"),
);
const FairnessVerifier = lazyWithReload(
  "fair",
  () => import("@/components/fairness/FairnessVerifier"),
);
const FriendsScreen = lazyWithReload(
  "friends",
  () => import("@/components/friends/FriendsScreen"),
);
const HistoryScreen = lazyWithReload(
  "history",
  () => import("@/components/history/HistoryScreen"),
);
const Nearby = lazyWithReload("nearby", () => import("@/components/nearby/Nearby"));

function Loading() {
  return (
    <div className="grid h-full place-items-center text-ink-faint">Loading…</div>
  );
}

/** Releases the seat server-side then tears down the local session. Shared by
 *  the back-button guard and TableMenu's "Leave table" so both paths free the
 *  seat identically. Reads the store imperatively (called outside render). */
function leaveTableAndTeardown(): void {
  const { transport, tableId, disconnect } = useGame.getState();
  if (transport && tableId) {
    transport.send({ type: Cmd.Leave, data: { tableId } });
  }
  disconnect();
}

const IS_NATIVE = Capacitor.isNativePlatform();

/** Android hardware/gesture back-button integration. Without this, back falls
 *  through to raw WebView history: it can pop a live table with no warning and
 *  does nothing at the root (silently suspending the app). Here back navigates
 *  within the app, guards leaving a live table, and exits only at the root. */
function useNativeBackButton(navigate: NavigateFunction): void {
  useEffect(() => {
    if (!IS_NATIVE) return;
    let remove: (() => void) | undefined;
    void CapApp.addListener("backButton", ({ canGoBack }) => {
      const path = window.location.pathname;
      if (path.startsWith("/table")) {
        const ok = window.confirm(
          "Leave the table? Your seat will be released.",
        );
        if (ok) {
          leaveTableAndTeardown();
          navigate("/lobby");
        }
        return;
      }
      if (path === "/" || path === "") {
        void CapApp.exitApp();
        return;
      }
      if (canGoBack) navigate(-1);
      else void CapApp.exitApp();
    }).then((handle) => {
      remove = () => void handle.remove();
    });
    return () => remove?.();
  }, [navigate]);
}

/** Deep links: an incoming /t/:joinCode URL (App Link / Universal Link / custom
 *  scheme) routes straight into the join flow. Registered domains live in the
 *  native projects; see MOBILE.md. */
function useDeepLinks(navigate: NavigateFunction): void {
  useEffect(() => {
    if (!IS_NATIVE) return;
    let remove: (() => void) | undefined;
    void CapApp.addListener("appUrlOpen", ({ url }) => {
      const target = joinPathFromUrl(url);
      if (target) navigate(target);
    }).then((handle) => {
      remove = () => void handle.remove();
    });
    return () => remove?.();
  }, [navigate]);
}

/** Extracts an in-app route from a deep-link URL. Handles both the App/Universal
 *  Link form (https://domain/t/CODE) and the custom-scheme form
 *  (com.felt.poker://t/CODE), which URL parsers treat inconsistently. */
function joinPathFromUrl(url: string): string | null {
  const match = /\/t\/([^/?#]+)/.exec(url);
  if (match) return `/t/${match[1]}`;
  return null;
}

/** Status-bar + soft-keyboard native setup, run once on native startup. Light
 *  content over the near-black (#0B0F14) surface; keyboard resizes the WebView
 *  body so inputs on auth/table are never covered. */
function useNativeChrome(): void {
  useEffect(() => {
    if (!IS_NATIVE) return;
    // Style.Dark = light text/icons for a dark background (Capacitor naming).
    void StatusBar.setStyle({ style: Style.Dark }).catch(() => {});
    if (Capacitor.getPlatform() === "android") {
      void StatusBar.setBackgroundColor({ color: "#0B0F14" }).catch(() => {});
    }
    void Keyboard.setResizeMode({ mode: KeyboardResize.Native }).catch(() => {});
  }, []);
}

/** Router-context wrapper that wires the native shell integrations and hosts the
 *  route-level ErrorBoundary + Suspense. Resets the boundary on navigation so a
 *  crash on one route does not stick after moving away. */
function NativeShell() {
  const navigate = useNavigate();
  const location = useLocation();
  useNativeBackButton(navigate);
  useDeepLinks(navigate);
  useNativeChrome();

  return (
    <ErrorBoundary label="route" resetKeys={[location.pathname]}>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route path="/" element={<Landing />} />
          <Route path="/t/:joinCode" element={<Landing />} />
          <Route path="/table" element={<TablePage />} />
          <Route path="/lobby" element={<Lobby />} />
          <Route path="/nearby" element={<Nearby />} />
          <Route path="/auth" element={<Auth />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/replay" element={<HandReplayer />} />
          <Route path="/fair" element={<FairnessVerifier />} />
          <Route path="/friends" element={<FriendsScreen />} />
          <Route path="/history" element={<HistoryScreen />} />
        </Routes>
      </Suspense>
    </ErrorBoundary>
  );
}

export default function App() {
  const theme = useSettings((s) => s.theme);
  const reducedMotion = useSettings((s) => s.reducedMotion);

  // Keep <html> classes in sync with persisted settings on load + change.
  useEffect(() => {
    applyDocumentClasses(theme, reducedMotion);
  }, [theme, reducedMotion]);

  // MotionConfig drives Framer Motion globally: "always" when the app's own
  // Reduced-motion toggle is on; otherwise "user" so the OS prefers-reduced-
  // motion setting is honored. Either source disables motion.
  return (
    <ErrorBoundary label="app-root">
      <MotionConfig reducedMotion={reducedMotion ? "always" : "user"}>
        <BrowserRouter>
          <NativeShell />
        </BrowserRouter>
      </MotionConfig>
    </ErrorBoundary>
  );
}
