import type { TrajRow } from "../state/trajectory";

export interface SpeedSummary {
  output: number;
  modelSeconds: number;
  average: number;
  /** Seconds until the latest measured round's first output; 0 when none was. */
  firstToken: number;
}

/** Derive only claims the trajectory can support. Durable event replay keeps
 * ordering but not original wall-clock timestamps, so a replayed begin/commit
 * pair collapses to milliseconds. Those rows still carry authoritative output
 * tokens, but they must not manufacture an average throughput figure. */
export function speedOf(rows: TrajRow[]): SpeedSummary {
  const lastTurn = rows.reduce((latest, row, index) => row.kind === "turn_started" ? index : latest, -1);
  const scope = lastTurn >= 0 ? rows.slice(lastTurn) : rows;
  const rounds = scope.filter((row) => row.kind === "model_round" && (row.dur ?? 0) > 0);
  const output = rounds.reduce((sum, row) => sum + (row.out ?? 0), 0);
  const timed = rounds.filter((row) => (row.dur ?? 0) >= 0.05 && (row.out ?? 0) > 0);
  const timedOutput = timed.reduce((sum, row) => sum + (row.out ?? 0), 0);
  const modelSeconds = timed.reduce((sum, row) => sum + (row.dur ?? 0), 0);
  const candidate = timedOutput > 0 && modelSeconds > 0 ? timedOutput / modelSeconds : 0;
  const firstToken = [...rounds].reverse().find((row) => (row.ttft ?? 0) > 0)?.ttft ?? 0;
  return {
    firstToken,
    output,
    modelSeconds,
    average: candidate > 0 && candidate <= 1000 ? candidate : 0,
  };
}
