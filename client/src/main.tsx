import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { ErrorBoundary } from "./components/error/ErrorBoundary";
import "./index.css";

// Outermost boundary: a last line of defense so even an error thrown while the
// app tree first mounts renders the branded fallback instead of a blank page.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary label="mount">
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
