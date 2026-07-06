// App-wide render-error boundary. A single uncaught render error must never
// white-screen the whole app - fatal inside the Capacitor shell, which has no
// browser chrome to reload from. This class catches the error, logs it, and
// renders a branded, self-contained fallback with a "Reload" action.
//
// Used twice: once around the whole app (last line of defense) and again around
// <Suspense> so a failed lazy chunk load is recoverable in-place. `resetKeys`
// lets a parent clear the error when navigation context changes.

import { Component, type ErrorInfo, type ReactNode } from "react";
import { isChunkLoadError } from "./chunkError";

interface Props {
  children: ReactNode;
  /** Optional label so nested boundaries log which region failed. */
  label?: string;
  /** When any value here changes, the boundary clears its error and retries. */
  resetKeys?: ReadonlyArray<unknown>;
  /** Custom fallback; falls back to the branded default when omitted. */
  fallback?: (error: Error, reset: () => void) => ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Server-side we would ship this to an error sink; here we log with enough
    // context to debug from a device console. Never swallow silently.
    const where = this.props.label ? ` [${this.props.label}]` : "";
    console.error(`ErrorBoundary${where} caught a render error:`, error, info);
  }

  componentDidUpdate(prev: Props): void {
    if (this.state.error && prev.resetKeys !== this.props.resetKeys) {
      if (!shallowEqualArrays(prev.resetKeys, this.props.resetKeys)) {
        this.reset();
      }
    }
  }

  reset = (): void => {
    this.setState({ error: null });
  };

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;
    if (this.props.fallback) return this.props.fallback(error, this.reset);
    return <ErrorFallback error={error} onReset={this.reset} />;
  }
}

function shallowEqualArrays(
  a: ReadonlyArray<unknown> | undefined,
  b: ReadonlyArray<unknown> | undefined,
): boolean {
  if (a === b) return true;
  if (!a || !b || a.length !== b.length) return false;
  return a.every((v, i) => Object.is(v, b[i]));
}

// A branded, dependency-free fallback. Inline styles only, so it renders even if
// the CSS bundle itself failed. A full reload is the surest recovery inside a
// packaged WebView; the parent-provided reset covers the recoverable cases.
function ErrorFallback({
  error,
  onReset,
}: {
  error: Error;
  onReset: () => void;
}) {
  const isChunkError = isChunkLoadError(error);
  return (
    <div
      role="alert"
      style={{
        minHeight: "100%",
        width: "100%",
        display: "grid",
        placeItems: "center",
        padding: "2rem",
        background: "#0B0F14",
        color: "#E6EBF0",
        textAlign: "center",
      }}
    >
      <div style={{ maxWidth: 360, display: "grid", gap: "1rem", justifyItems: "center" }}>
        <svg width={44} height={44} viewBox="0 0 48 48" fill="none" aria-hidden>
          <circle cx="24" cy="24" r="22.5" stroke="#D4A64A" strokeWidth="1.5" opacity="0.55" />
          <path
            d="M24 10c5.2 5 11 9.6 11 15.2 0 3.4-2.4 5.6-5.3 5.6-1.7 0-3.2-.8-4.2-2.1.4 2.6 1.5 5 3.5 6.6h-10c2-1.6 3.1-4 3.5-6.6-1 1.3-2.5 2.1-4.2 2.1-2.9 0-5.3-2.2-5.3-5.6C13 19.6 18.8 15 24 10Z"
            fill="#D4A64A"
          />
        </svg>
        <h1 style={{ fontSize: "1.25rem", fontWeight: 600, margin: 0 }}>
          {isChunkError ? "Update available" : "Something went wrong"}
        </h1>
        <p style={{ fontSize: "0.9rem", lineHeight: 1.5, color: "#9AA6B2", margin: 0 }}>
          {isChunkError
            ? "This screen could not load - the app was likely updated. Reload to get the latest version."
            : "The app hit an unexpected error. You can reload without losing your place at the table."}
        </p>
        <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.25rem" }}>
          <button
            onClick={() => window.location.reload()}
            style={{
              minHeight: 44,
              padding: "0 1.25rem",
              borderRadius: 12,
              border: "none",
              background: "#D4A64A",
              color: "#231704",
              fontWeight: 600,
              fontSize: "0.95rem",
            }}
          >
            Reload
          </button>
          {!isChunkError && (
            <button
              onClick={onReset}
              style={{
                minHeight: 44,
                padding: "0 1.25rem",
                borderRadius: 12,
                border: "1px solid rgba(255,255,255,0.16)",
                background: "transparent",
                color: "#9AA6B2",
                fontWeight: 600,
                fontSize: "0.95rem",
              }}
            >
              Try again
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
