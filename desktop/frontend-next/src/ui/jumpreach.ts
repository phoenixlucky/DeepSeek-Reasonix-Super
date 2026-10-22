export const EDGE = 48;

export type Direction = "up" | "down";

export function reach(top: number, client: number, height: number): { up: boolean; down: boolean } {
  if (height <= client) return { up: false, down: false };
  return { up: top >= EDGE, down: height - client - top >= EDGE };
}

// Tops are positions in the scroller's content. The nearest block beyond the
// view in the chosen direction is the landing; with none, the whole end.
export function landing(dir: Direction, tops: number[], top: number, client: number, height: number): number | "end" {
  const ahead = dir === "up" ? tops.filter((at) => at < top - 4) : tops.filter((at) => at > top + client - 4);
  if (ahead.length === 0) return dir === "up" ? 0 : "end";
  const at = Math.max(0, (dir === "up" ? Math.max(...ahead) : Math.min(...ahead)) - 8);
  return dir === "down" && height - client - at < EDGE ? "end" : at;
}
