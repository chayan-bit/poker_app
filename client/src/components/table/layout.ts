// ONE table, two projections. Seats live on a single normalized ring indexed
// clockwise from the hero (bottom-center). We rotate the ring so the local
// player is always at the bottom, then project each ring position onto an
// ellipse whose radii come from the container aspect ratio:
//   - portrait  -> tall, narrow ellipse (vertical table)
//   - landscape -> wide oval (classic table)
// The SAME seat coordinates feed both; we never build two tables.

export interface Point {
  x: number;
  y: number;
}

export interface SeatSlot extends Point {
  seat: number;
  /** where this seat's chips travel to reach the pot (toward center) */
  toPot: Point;
}

export interface LayoutInput {
  width: number;
  height: number;
  seatIndices: number[]; // occupied seat numbers, ascending
  heroSeat: number | null;
}

export function computeLayout({
  width,
  height,
  seatIndices,
  heroSeat,
}: LayoutInput): { slots: SeatSlot[]; center: Point } {
  const center: Point = { x: width / 2, y: height / 2 };
  const n = seatIndices.length;
  if (n === 0 || width === 0 || height === 0)
    return { slots: [], center };

  const portrait = height >= width;
  // Ellipse radii: leave room for avatars + action bar at the bottom.
  const rx = width * (portrait ? 0.36 : 0.42);
  const ry = height * (portrait ? 0.34 : 0.36);

  // Put the hero at the bottom (angle = 90deg = PI/2). Others fan around.
  const heroPos =
    heroSeat === null ? 0 : Math.max(0, seatIndices.indexOf(heroSeat));

  const slots: SeatSlot[] = seatIndices.map((seat, i) => {
    // ring position relative to hero, clockwise
    const rel = (i - heroPos + n) % n;
    const t = rel / n; // 0..1
    // angle: start at bottom (PI/2) and go clockwise (increase angle)
    const angle = Math.PI / 2 + t * Math.PI * 2;
    const x = center.x + rx * Math.cos(angle);
    const y = center.y + ry * Math.sin(angle);
    // chip target: 82% of the way toward center
    const toPot: Point = {
      x: (center.x - x) * 0.82,
      y: (center.y - y) * 0.82,
    };
    return { seat, x, y, toPot };
  });

  return { slots, center };
}
