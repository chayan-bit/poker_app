// The table's motion vocabulary. Every primitive is transform/opacity only
// (GPU-composited), driven by the Web Animations API - NOT Framer Motion, NOT a
// requestAnimationFrame loop - so the main thread stays free for the socket.
//
// Only these animate during a hand:
//   deal, flip, chip-bet, pot-merge, pot-push, fold-muck, timer-pulse, win-glow
//
// Durations read the CSS tokens so `prefers-reduced-motion` / the in-app toggle
// collapse them to 80ms fades in one place.

function dur(token: string, fallback: number): number {
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(token)
    .trim();
  const ms = parseFloat(raw);
  return Number.isFinite(ms) ? ms : fallback;
}

const EASE = "cubic-bezier(0.22, 1, 0.36, 1)";

function reduced(): boolean {
  return (
    document.documentElement.classList.contains("reduce-motion") ||
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

interface Vec {
  x: number;
  y: number;
}

/** Slide a node from `from` (in px, relative to its rest position) to rest. */
export function deal(el: HTMLElement, from: Vec, delay = 0): Animation {
  const d = dur("--dur", 200);
  if (reduced()) return fade(el, delay);
  return el.animate(
    [
      { transform: `translate(${from.x}px, ${from.y}px) scale(0.9)`, opacity: 0 },
      { transform: "translate(0,0) scale(1)", opacity: 1 },
    ],
    { duration: d, easing: EASE, delay, fill: "backwards" },
  );
}

/** Card flip via rotateY; content swap is the caller's job at the midpoint. */
export function flip(el: HTMLElement, onMid?: () => void): Animation {
  const d = dur("--dur", 200);
  if (reduced()) {
    onMid?.();
    return fade(el);
  }
  const anim = el.animate(
    [
      { transform: "rotateY(0deg)" },
      { transform: "rotateY(90deg)" },
      { transform: "rotateY(0deg)" },
    ],
    { duration: d, easing: EASE },
  );
  const half = d / 2;
  window.setTimeout(() => onMid?.(), half);
  return anim;
}

/** Chip moves from a seat toward the pot when a bet is placed. */
export function chipBet(el: HTMLElement, to: Vec): Animation {
  const d = dur("--dur", 200);
  if (reduced()) return fade(el);
  return el.animate(
    [
      { transform: "translate(0,0)", opacity: 1 },
      { transform: `translate(${to.x}px, ${to.y}px)`, opacity: 0.9 },
    ],
    { duration: d, easing: EASE, fill: "forwards" },
  );
}

/** Street-end: bets slide into the central pot. */
export function potMerge(el: HTMLElement, to: Vec): Animation {
  const d = dur("--dur", 200);
  if (reduced()) return fade(el);
  return el.animate(
    [
      { transform: "translate(0,0)", opacity: 1 },
      { transform: `translate(${to.x}px, ${to.y}px) scale(0.8)`, opacity: 0 },
    ],
    { duration: d, easing: EASE, fill: "forwards" },
  );
}

/** The payoff: pot travels to the winning seat. Allowed up to 400ms. */
export function potPush(el: HTMLElement, to: Vec): Animation {
  const d = dur("--dur-pot", 400);
  if (reduced()) return fade(el);
  return el.animate(
    [
      { transform: "translate(0,0) scale(1)", opacity: 1 },
      { transform: `translate(${to.x}px, ${to.y}px) scale(0.7)`, opacity: 0 },
    ],
    { duration: d, easing: EASE, fill: "forwards" },
  );
}

/** A folded hand mucks: fade + slight downward drift. */
export function foldMuck(el: HTMLElement): Animation {
  const d = dur("--dur-fast", 150);
  return el.animate(
    [
      { transform: "translateY(0) scale(1)", opacity: 1 },
      { transform: "translateY(10px) scale(0.94)", opacity: 0.15 },
    ],
    { duration: d, easing: EASE, fill: "forwards" },
  );
}

/** Timer ring pulse in the final seconds. Scale only; no full-screen flashing. */
export function timerPulse(el: HTMLElement): Animation {
  if (reduced()) return fade(el, 0, 0);
  return el.animate(
    [{ transform: "scale(1)" }, { transform: "scale(1.06)" }, { transform: "scale(1)" }],
    { duration: 900, easing: "ease-in-out", iterations: Infinity },
  );
}

/** Winner glow: opacity pulse on a halo element behind the seat. */
export function winGlow(el: HTMLElement): Animation {
  const d = dur("--dur-pot", 400);
  if (reduced()) return fade(el);
  return el.animate(
    [
      { opacity: 0, transform: "scale(0.9)" },
      { opacity: 1, transform: "scale(1.04)" },
      { opacity: 0.6, transform: "scale(1)" },
    ],
    { duration: d, easing: EASE, fill: "forwards" },
  );
}

function fade(el: HTMLElement, delay = 0, to = 1): Animation {
  return el.animate([{ opacity: 0 }, { opacity: to }], {
    duration: 80,
    delay,
    fill: "backwards",
  });
}
