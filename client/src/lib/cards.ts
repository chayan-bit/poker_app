// Card parsing + rendering helpers. Cards are strings like "As", "Td", "2c".

import type { Card, Suit } from "@/net/protocol";

export interface ParsedCard {
  rank: string; // "A" "K" "Q" "J" "T" "9".."2"
  suit: Suit;
}

const SUIT_GLYPH: Record<Suit, string> = {
  s: "♠", // ♠
  h: "♥", // ♥
  d: "♦", // ♦
  c: "♣", // ♣
};

const SUIT_NAME: Record<Suit, string> = {
  s: "spades",
  h: "hearts",
  d: "diamonds",
  c: "clubs",
};

const RANK_NAME: Record<string, string> = {
  A: "Ace",
  K: "King",
  Q: "Queen",
  J: "Jack",
  T: "Ten",
  "9": "Nine",
  "8": "Eight",
  "7": "Seven",
  "6": "Six",
  "5": "Five",
  "4": "Four",
  "3": "Three",
  "2": "Two",
};

export function parseCard(card: Card): ParsedCard {
  return { rank: card.slice(0, 1), suit: card.slice(1, 2) as Suit };
}

export function suitGlyph(suit: Suit): string {
  return SUIT_GLYPH[suit];
}

/** Screen-reader label, e.g. "Ace of spades". */
export function cardLabel(card: Card): string {
  const { rank, suit } = parseCard(card);
  return `${RANK_NAME[rank] ?? rank} of ${SUIT_NAME[suit]}`;
}

// Two color modes. Two-color: red for h/d, ink for s/c. Four-color: each suit a
// distinct hue so suits are distinguishable WITHOUT relying on red/black alone.
export function suitColor(suit: Suit, fourColor: boolean): string {
  // Concrete colors: card faces are white in both themes, and SVG presentation
  // attributes don't resolve CSS var(), so these must be literal.
  if (fourColor) {
    switch (suit) {
      case "s":
        return "#14304a"; // deep blue-black spades
      case "h":
        return "#c93838";
      case "d":
        return "#2f7fd6";
      case "c":
        return "#1f9d55";
    }
  }
  return suit === "h" || suit === "d" ? "#c93838" : "#14181d";
}
