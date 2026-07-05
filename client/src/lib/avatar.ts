// Deterministic avatar gradient from a name/seat, so every player reads as a
// distinct token without shipping photo assets. Two harmonious hues on the
// dark felt; the monogram sits on top.

export function avatarGradient(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) % 360;
  const h2 = (h + 40) % 360;
  return `linear-gradient(140deg, oklch(62% 0.12 ${h}), oklch(46% 0.14 ${h2}))`;
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 1) return parts[0].slice(0, 1).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}
