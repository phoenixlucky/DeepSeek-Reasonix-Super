// The share of max that value fills, as a whole percentage held between 0 and
// 100. A missing or non-positive max means there is nothing to measure against.
export function clampedShare(value: number, max: number): number {
  if (!(max > 0) || !Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, Math.round((value / max) * 100)));
}
