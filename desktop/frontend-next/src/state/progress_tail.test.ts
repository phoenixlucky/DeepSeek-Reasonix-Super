import { describe, expect, it } from "vitest";
import { foldProgress, foldTool, PROGRESS_TAIL } from "./fold";
import type { Item } from "./session";

const start = (): Item[] => foldTool([], { id: "t1", name: "bash", args: "{}", readOnly: false }, true);
const row = (items: Item[]) => items[0] as Extract<Item, { t: "tool" }>;
const feed = (chunks: string[]) => chunks.reduce((items, c) => foldProgress(items, { id: "t1", name: "bash", output: c, readOnly: false }), start());

describe("the output a running tool shows", () => {
  it("is the whole of it while it is small, and says nothing was left out", () => {
    const r = row(feed(["one\n", "two\n"]));
    expect(r.tool.output).toBe("one\ntwo\n");
    expect(r.omitted ?? 0).toBe(0);
  });

  it("is only its last 64 KB however much it has printed, and counts what came before", () => {
    const line = "x".repeat(99) + "\n";
    const chunks = Array.from({ length: 3000 }, () => line.repeat(10));
    const r = row(feed(chunks));
    expect(r.tool.output!.length).toBeLessThanOrEqual(PROGRESS_TAIL);
    expect(r.tool.output!.endsWith(line)).toBe(true);
    expect(r.omitted! + r.tool.output!.length).toBe(3000 * 1000);
  });

  it("never cuts a character in half when it trims", () => {
    const emoji = "😀";
    const r = row(feed([emoji.repeat(PROGRESS_TAIL), "a"]));
    const first = r.tool.output!.charCodeAt(0);
    expect(first >= 0xdc00 && first <= 0xdfff).toBe(false);
    expect(r.tool.output!.endsWith("a")).toBe(true);
  });

  it("is replaced by the result when the call returns, with nothing marked as omitted", () => {
    const grown = feed(["y".repeat(PROGRESS_TAIL + 5000)]);
    expect(row(grown).omitted).toBeGreaterThan(0);
    const done = foldTool(grown, { id: "t1", name: "bash", output: "final", readOnly: false }, false);
    expect(row(done).tool.output).toBe("final");
    expect(row(done).omitted).toBeUndefined();
  });
});
