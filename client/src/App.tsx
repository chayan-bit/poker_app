// App shell + routing. The table view is eagerly imported (priority bundle);
// everything else is lazy-loaded so the table renders first.

import { lazy, Suspense, useEffect } from "react";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import TablePage from "@/components/table/TablePage";
import Landing from "@/components/landing/Landing";
import { applyDocumentClasses, useSettings } from "@/store/settingsStore";

const Lobby = lazy(() => import("@/components/lobby/Lobby"));
const Auth = lazy(() => import("@/components/auth/Auth"));
const Settings = lazy(() => import("@/components/settings/Settings"));
const HandReplayer = lazy(() => import("@/components/replay/HandReplayer"));
const FairnessVerifier = lazy(
  () => import("@/components/fairness/FairnessVerifier"),
);
const FriendsScreen = lazy(() => import("@/components/friends/FriendsScreen"));
const HistoryScreen = lazy(() => import("@/components/history/HistoryScreen"));

function Loading() {
  return (
    <div className="grid h-full place-items-center text-ink-faint">Loading…</div>
  );
}

export default function App() {
  const theme = useSettings((s) => s.theme);
  const reducedMotion = useSettings((s) => s.reducedMotion);

  // Keep <html> classes in sync with persisted settings on load + change.
  useEffect(() => {
    applyDocumentClasses(theme, reducedMotion);
  }, [theme, reducedMotion]);

  return (
    <BrowserRouter>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route path="/" element={<Landing />} />
          <Route path="/t/:joinCode" element={<Landing />} />
          <Route path="/table" element={<TablePage />} />
          <Route path="/lobby" element={<Lobby />} />
          <Route path="/auth" element={<Auth />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/replay" element={<HandReplayer />} />
          <Route path="/fair" element={<FairnessVerifier />} />
          <Route path="/friends" element={<FriendsScreen />} />
          <Route path="/history" element={<HistoryScreen />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}
