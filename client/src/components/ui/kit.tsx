// Small shared UI kit for the non-table chrome (lobby, auth, settings). These
// screens MAY use Framer Motion; the table must not.

import type { ButtonHTMLAttributes, PropsWithChildren, ReactNode } from "react";

// Screen paints the ambient card-room backdrop, then lays content above it.
// `wide` opts into a roomier column for hero/lobby layouts.
export function Screen({
  children,
  title,
  back,
  wide,
}: PropsWithChildren<{ title?: string; back?: ReactNode; wide?: boolean }>) {
  return (
    <div className="ambient relative min-h-full w-full overflow-x-hidden">
      <div
        className={`relative z-10 mx-auto flex min-h-full w-full flex-col gap-5 ${wide ? "max-w-5xl" : "max-w-xl"}`}
        style={{
          paddingTop: "max(1.5rem, env(safe-area-inset-top))",
          paddingBottom: "max(1.5rem, env(safe-area-inset-bottom))",
          // Clear the notch/rounded corners in landscape too, not just top/bottom.
          paddingLeft: "max(1.25rem, env(safe-area-inset-left))",
          paddingRight: "max(1.25rem, env(safe-area-inset-right))",
        }}
      >
        {(title || back) && (
          <header className="flex items-center gap-3">
            {back}
            {title && (
              <h1 className="display text-[1.7rem]">{title}</h1>
            )}
          </header>
        )}
        {children}
      </div>
    </div>
  );
}

export function Card({
  children,
  className = "",
}: PropsWithChildren<{ className?: string }>) {
  return <div className={`card-edge rounded-2xl p-4 ${className}`}>{children}</div>;
}

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost" | "gold";
};

export function Button({ variant = "primary", className = "", ...rest }: BtnProps) {
  // One loud voice per screen: gold is the house CTA, primary is a quiet
  // filled neutral, ghost is chrome. Blue stays reserved for in-table action.
  const styles =
    variant === "gold"
      ? {
          background: "var(--gold)",
          color: "#231704",
          boxShadow:
            "var(--shadow-2), inset 0 1px 0 rgba(255,255,255,0.35), inset 0 -1px 0 rgba(0,0,0,0.18)",
        }
      : variant === "primary"
        ? {
            background: "var(--surface-4)",
            color: "var(--ink)",
            boxShadow:
              "var(--shadow-1), inset 0 1px 0 rgba(255,255,255,0.09), inset 0 0 0 1px var(--line-hi)",
          }
        : {
            background: "transparent",
            color: "var(--ink-dim)",
            boxShadow: "inset 0 0 0 1px var(--line)",
          };
  return (
    <button
      {...rest}
      style={styles}
      className={`no-tap-highlight min-h-[52px] rounded-xl px-5 text-base font-semibold tracking-tight transition-transform duration-150 ease-out will-change-transform hover:-translate-y-[1px] active:translate-y-[1px] active:scale-[0.99] disabled:pointer-events-none disabled:opacity-50 ${className}`}
    />
  );
}

export function Field({ label, children }: PropsWithChildren<{ label: string }>) {
  return (
    <label className="flex flex-col gap-1.5 text-sm">
      <span className="text-ink-dim">{label}</span>
      {children}
    </label>
  );
}

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`min-h-[50px] rounded-xl border border-line bg-surface px-3.5 text-base text-ink outline-none transition-shadow placeholder:text-ink-faint focus:border-action-blue focus:shadow-[0_0_0_3px_rgba(76,154,232,0.25)] ${props.className ?? ""}`}
    />
  );
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: () => void;
  label: string;
}) {
  return (
    <button
      onClick={onChange}
      role="switch"
      aria-checked={checked}
      className="no-tap-highlight flex w-full items-center justify-between py-2 text-left"
    >
      <span className="text-base">{label}</span>
      <span
        className="relative h-6 w-11 rounded-full transition-colors duration-200"
        style={{ background: checked ? "var(--action-blue)" : "var(--surface-4)" }}
      >
        <span
          className="absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform duration-200"
          style={{ transform: checked ? "translateX(22px)" : "translateX(2px)" }}
        />
      </span>
    </button>
  );
}

// Brand mark: a spade inside a fine ring. Used on the felt center, the landing
// badge, and anywhere the app signs its name. Never render the name in
// lowercase code-style ("poker_app") in UI - use <Wordmark/>.
export function SpadeMark({ size = 28, color = "currentColor" }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 48 48" fill="none" aria-hidden>
      <circle cx="24" cy="24" r="22.5" stroke={color} strokeWidth="1.5" opacity="0.55" />
      <path
        d="M24 10c5.2 5 11 9.6 11 15.2 0 3.4-2.4 5.6-5.3 5.6-1.7 0-3.2-.8-4.2-2.1.4 2.6 1.5 5 3.5 6.6h-10c2-1.6 3.1-4 3.5-6.6-1 1.3-2.5 2.1-4.2 2.1-2.9 0-5.3-2.2-5.3-5.6C13 19.6 18.8 15 24 10Z"
        fill={color}
      />
    </svg>
  );
}

export function Wordmark({ size = 20 }: { size?: number }) {
  return (
    <span className="inline-flex items-baseline gap-2">
      <span className="display" style={{ fontSize: size, fontWeight: 620 }}>
        Felt
      </span>
      <span
        className="num text-[0.6em] font-medium uppercase"
        style={{ letterSpacing: "0.32em", color: "var(--ink-faint)" }}
      >
        Poker
      </span>
    </span>
  );
}

// A shared monoline icon set (1.75 stroke, currentColor) - no emoji.
export function Icon({ name, size = 20 }: { name: "bolt" | "shield" | "devices" | "gear"; size?: number }) {
  const common = {
    width: size,
    height: size,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.75,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };
  if (name === "bolt")
    return (
      <svg {...common}>
        <path d="M13 2 4 14h7l-1 8 9-12h-7l1-8Z" />
      </svg>
    );
  if (name === "gear")
    return (
      <svg {...common}>
        <circle cx="12" cy="12" r="3.2" />
        <path d="M12 2.8v2.6M12 18.6v2.6M2.8 12h2.6M18.6 12h2.6M5.5 5.5l1.9 1.9M16.6 16.6l1.9 1.9M18.5 5.5l-1.9 1.9M7.4 16.6l-1.9 1.9" />
      </svg>
    );
  if (name === "shield")
    return (
      <svg {...common}>
        <path d="M12 3l7 3v5c0 5-3.5 8-7 10-3.5-2-7-5-7-10V6l7-3Z" />
        <path d="M9 12l2 2 4-4" />
      </svg>
    );
  return (
    <svg {...common}>
      <rect x="2.5" y="4" width="13" height="10" rx="1.5" />
      <rect x="16.5" y="8" width="5" height="12" rx="1.5" />
      <path d="M6 18h6" />
    </svg>
  );
}
