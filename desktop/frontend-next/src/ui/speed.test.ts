import { describe, expect, it } from "vitest";
import type { TrajRow } from "../state/trajectory";
import { speedOf } from "./speed";

const round = (dur: number, out: number): TrajRow => ({ seq: 1, at: 0, kind: "model_round", payload: [], subs: [], dur, out });

describe("generation speed summary", () => {
  it("reports the latest round's time to first token, and none when no round measured one", () => {
    const first = { ...round(2, 80), ttft: 0.9 };
    const second = { ...round(3, 90), seq: 2, ttft: 1.4 };
    expect(speedOf([first, second]).firstToken).toBe(1.4);
    expect(speedOf([first, { ...round(3, 90), seq: 2 }]).firstToken).toBe(0.9);
    expect(speedOf([round(2, 80)]).firstToken).toBe(0);
  });

  it("uses provider output tokens over observed model time", () => {
    expect(speedOf([round(2, 80), { ...round(3, 90), seq: 2 }])).toMatchObject({ output: 170, modelSeconds: 5, average: 34 });
  });

  it("keeps real output but refuses replay-collapsed time", () => {
    expect(speedOf([round(0.001, 420)])).toMatchObject({ output: 420, modelSeconds: 0, average: 0 });
  });

  it("ignores tools when calculating model throughput", () => {
    const tool: TrajRow = { seq: 2, at: 1, kind: "tool", payload: [], subs: [], dur: 9, out: 999, tool: "bash" };
    expect(speedOf([round(2, 80), tool])).toMatchObject({ output: 80, modelSeconds: 2, average: 40 });
  });

  it("describes the latest turn instead of accumulating the whole conversation", () => {
    const start: TrajRow = { seq: 2, at: 3, kind: "turn_started", payload: [], subs: [] };
    expect(speedOf([round(2, 80), start, { ...round(3, 90), seq: 3 }])).toMatchObject({ output: 90, modelSeconds: 3, average: 30 });
  });
});
