// A single playing card, hand-drawn as a self-contained SVG so it stays crisp
// at any size and bundles with no external assets. Suits are drawn as SVG
// PATHS, never Unicode glyphs: on iOS/Safari (and some Android builds) the ♥/♦
// code points default to emoji presentation and render as colored emoji,
// ignoring fill and breaking the two/four-color system. Paths render
// identically everywhere and honor the fill color.
//
// Layout follows the classic public-domain decks (Bellot / RevK): oversized
// corner index, a proper pip matrix for number cards, and clean monogram court
// cards - legibility over ornament. Face-down cards show a woven back.
//
// The card is a persistent node; flips run via the WAAPI `flip` primitive
// triggered by the parent when the face changes.

import { memo, useId } from "react";
import type { Card as CardT, Suit } from "@/net/protocol";
import { parseCard, suitColor, cardLabel } from "@/lib/cards";
import { useSettings } from "@/store/settingsStore";

interface Props {
  card?: CardT; // undefined => face down
  size?: "sm" | "md" | "lg";
  dimmed?: boolean;
}

// width in px; height derived at the poker 1:1.4 ratio.
const W = { sm: 34, md: 50, lg: 66 } as const;

// Suit shapes in a 0..100 box. Heart/diamond/spade are single paths; club is
// composed of three lobes + a stem.
const SUIT_PATH: Record<Exclude<Suit, "c">, string> = {
  h: "M50 84 C26 66 12 51 12 34 C12 21 21 13 32 13 C41 13 47 19 50 26 C53 19 59 13 68 13 C79 13 88 21 88 34 C88 51 74 66 50 84 Z",
  d: "M50 6 L86 50 L50 94 L14 50 Z",
  s: "M50 8 C50 8 88 33 88 55 C88 68 79 76 69 76 C63 76 58 73 54 68 C55 77 59 86 68 92 L32 92 C41 86 45 77 46 68 C42 73 37 76 31 76 C21 76 12 68 12 55 C12 33 50 8 50 8 Z",
};

function SuitShape({
  suit,
  cx,
  cy,
  size,
  color,
  flip = false,
}: {
  suit: Suit;
  cx: number;
  cy: number;
  size: number;
  color: string;
  flip?: boolean;
}) {
  const s = size / 100;
  return (
    <g transform={`translate(${cx - size / 2} ${cy - size / 2}) scale(${s})`}>
      <g transform={flip ? "rotate(180 50 50)" : undefined} fill={color}>
        {suit === "c" ? (
          <>
            <circle cx={50} cy={30} r={18} />
            <circle cx={29} cy={55} r={18} />
            <circle cx={71} cy={55} r={18} />
            <path d="M43 50 L57 50 L63 92 L37 92 Z" />
          </>
        ) : (
          <path d={SUIT_PATH[suit as Exclude<Suit, "c">]} />
        )}
      </g>
    </g>
  );
}

// Pip layout per rank, as [col, row] on a grid (rows 0..6, cols 0..2).
const PIPS: Record<string, [number, number][]> = {
  "2": [[1, 0], [1, 6]],
  "3": [[1, 0], [1, 3], [1, 6]],
  "4": [[0, 0], [2, 0], [0, 6], [2, 6]],
  "5": [[0, 0], [2, 0], [1, 3], [0, 6], [2, 6]],
  "6": [[0, 0], [2, 0], [0, 3], [2, 3], [0, 6], [2, 6]],
  "7": [[0, 0], [2, 0], [1, 1.5], [0, 3], [2, 3], [0, 6], [2, 6]],
  "8": [[0, 0], [2, 0], [1, 1.5], [0, 3], [2, 3], [1, 4.5], [0, 6], [2, 6]],
  "9": [[0, 0], [2, 0], [0, 2], [2, 2], [1, 3], [0, 4], [2, 4], [0, 6], [2, 6]],
  T: [[0, 0], [2, 0], [0, 2], [2, 2], [1, 1], [1, 5], [0, 4], [2, 4], [0, 6], [2, 6]],
};

const COURT = new Set(["J", "Q", "K"]);

function CardFace({ card, w }: { card: CardT; w: number }) {
  const fourColor = useSettings((s) => s.fourColorDeck);
  const { rank, suit } = parseCard(card);
  const color = suitColor(suit as Suit, fourColor);
  const h = w * 1.4;

  // pip grid geometry
  const gx = [w * 0.3, w * 0.5, w * 0.7];
  const gy0 = h * 0.24;
  const gy6 = h * 0.76;
  const gyStep = (gy6 - gy0) / 6;
  const pipSize = w * 0.19;

  return (
    <svg
      width={w}
      height={h}
      viewBox={`0 0 ${w} ${h}`}
      role="img"
      aria-label={cardLabel(card)}
      className="block"
      style={{ borderRadius: w * 0.12 }}
    >
      <rect
        x={0.5}
        y={0.5}
        width={w - 1}
        height={h - 1}
        rx={w * 0.12}
        fill="#f8fafc"
        stroke="rgba(0,0,0,0.14)"
        strokeWidth={1}
      />

      {/* corner index (top-left) + mirrored (bottom-right) */}
      <CornerIndex rank={rank} suit={suit as Suit} color={color} w={w} h={h} />
      <g transform={`rotate(180 ${w / 2} ${h / 2})`}>
        <CornerIndex rank={rank} suit={suit as Suit} color={color} w={w} h={h} />
      </g>

      {/* center */}
      {rank === "A" ? (
        <SuitShape suit={suit as Suit} cx={w / 2} cy={h / 2} size={w * 0.5} color={color} />
      ) : (
        (PIPS[rank] ?? []).map(([c, r], i) => (
          <SuitShape
            key={i}
            suit={suit as Suit}
            cx={gx[c]}
            cy={gy0 + r * gyStep}
            size={pipSize}
            color={color}
            flip={r > 3}
          />
        ))
      )}
    </svg>
  );
}

// Corner: rank on a hanging baseline (never clips the top edge) + a small suit
// shape below it, both nudged in from the corner.
function CornerIndex({
  rank,
  suit,
  color,
  w,
  h,
}: {
  rank: string;
  suit: Suit;
  color: string;
  w: number;
  h: number;
}) {
  const cx = w * 0.17;
  const rankY = h * 0.055;
  return (
    <g>
      <text
        x={cx}
        y={rankY}
        textAnchor="middle"
        dominantBaseline="hanging"
        fontSize={w * 0.3}
        fontWeight={800}
        fill={color}
        style={{ fontFamily: "Inter, sans-serif" }}
      >
        {rank}
      </text>
      <SuitShape suit={suit} cx={cx} cy={rankY + w * 0.36} size={w * 0.19} color={color} />
    </g>
  );
}

// Court cards (J/Q/K) use real public-domain court artwork (Aguilar/Bellot
// vector deck, public domain) dropped into our white card frame. The artwork
// already carries its own traditional corner indices, so we don't overlay ours.
function CourtImg({ card, w }: { card: CardT; w: number }) {
  const h = w * 1.4;
  const { rank, suit } = parseCard(card);
  const src = `${import.meta.env.BASE_URL}cards/${rank}${suit}.svg`;
  return (
    <div
      role="img"
      aria-label={cardLabel(card)}
      style={{
        width: w,
        height: h,
        borderRadius: w * 0.12,
        background: "#f8fafc",
        border: "1px solid rgba(0,0,0,0.14)",
        overflow: "hidden",
      }}
    >
      <img
        src={src}
        alt=""
        draggable={false}
        style={{
          width: "100%",
          height: "100%",
          objectFit: "contain",
          display: "block",
          padding: w * 0.05,
        }}
      />
    </div>
  );
}

function CardBack({ w }: { w: number }) {
  const h = w * 1.4;
  const uid = useId().replace(/:/g, ""); // unique, url()-safe ids per instance
  const grad = `cb-${uid}`;
  const weave = `wv-${uid}`;
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} aria-label="face-down card" className="block">
      <defs>
        <linearGradient id={grad} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#1c3a5e" />
          <stop offset="1" stopColor="#14283f" />
        </linearGradient>
        <pattern id={weave} width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
          <rect width="8" height="8" fill={`url(#${grad})`} />
          <path d="M0 4 H8 M4 0 V8" stroke="rgba(255,255,255,0.06)" strokeWidth="1" />
        </pattern>
      </defs>
      <rect x={0.5} y={0.5} width={w - 1} height={h - 1} rx={w * 0.12} fill={`url(#${weave})`} stroke="rgba(0,0,0,0.35)" />
      <rect
        x={w * 0.12}
        y={h * 0.08}
        width={w * 0.76}
        height={h * 0.84}
        rx={w * 0.08}
        fill="none"
        stroke="rgba(232,180,76,0.35)"
        strokeWidth={1}
      />
    </svg>
  );
}

function CardImpl({ card, size = "md", dimmed = false }: Props) {
  const w = W[size];
  const isCourt = !!card && COURT.has(card.slice(0, 1));
  return (
    <div
      className="gpu no-tap-highlight"
      style={{
        opacity: dimmed ? 0.42 : 1,
        filter: dimmed ? "grayscale(0.4)" : "none",
        boxShadow: "var(--shadow-2)",
        borderRadius: w * 0.12,
        transition: "opacity var(--dur) var(--ease)",
      }}
    >
      {card ? (
        isCourt ? (
          <CourtImg card={card} w={w} />
        ) : (
          <CardFace card={card} w={w} />
        )
      ) : (
        <CardBack w={w} />
      )}
    </div>
  );
}

export const Card = memo(CardImpl);
