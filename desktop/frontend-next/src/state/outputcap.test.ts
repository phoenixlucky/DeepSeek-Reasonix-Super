import { describe, expect, it } from "vitest";
import { capToolOutput, OUTPUT_CAP, OUTPUT_EDGE } from "./outputcap";
import type { Tool } from "../port/wire";

const tool = (output: string, over: Partial<Tool> = {}): Tool => ({ id: "c1", name: "bash", args: "{}", output, readOnly: false, ...over });

describe("what the page keeps of a finished call's output", () => {
  it("is all of it up to the cap", () => {
    const t = tool("x".repeat(OUTPUT_CAP));
    expect(capToolOutput(t)).toBe(t);
  });

  it("is the first and last 4 KB beyond it, counting the middle it let go of", () => {
    const body = "a".repeat(OUTPUT_EDGE) + "m".repeat(20000) + "z".repeat(OUTPUT_EDGE);
    const capped = capToolOutput(tool(body));
    expect(capped.output).toBe("a".repeat(OUTPUT_EDGE) + "\n⋯\n" + "z".repeat(OUTPUT_EDGE));
    expect(capped.hidden).toBe(20000);
    expect(capped.output!.length + capped.hidden! - 3).toBe(body.length);
  });

  it("cuts no character in half at either end", () => {
    const emoji = "😀";
    const body = "a".repeat(OUTPUT_EDGE - 1) + emoji + "m".repeat(20000) + emoji + "b".repeat(OUTPUT_EDGE - 1);
    const out = capToolOutput(tool(body)).output!;
    expect(/[\ud800-\udbff]$/.test(out.split("\n⋯\n")[0])).toBe(false);
    expect(/^[\udc00-\udfff]/.test(out.split("\n⋯\n")[1])).toBe(false);
  });

  it("leaves a diff, a plan and a remembered fact whole, because something else reads them", () => {
    const big = "x".repeat(OUTPUT_CAP * 2);
    expect(capToolOutput(tool(big, { outputDiff: true })).output).toBe(big);
    expect(capToolOutput(tool(big, { name: "todo_write" })).output).toBe(big);
    expect(capToolOutput(tool(big, { name: "remember" })).output).toBe(big);
  });

  it("leaves a lookup whole, because the manifest card counts its lines and hits from the text", () => {
    const lines = Array.from({ length: 2000 }, (_, i) => `line ${i}`).join("\n");
    for (const name of ["read_file", "grep", "glob", "ls"]) expect(capToolOutput(tool(lines, { name })).output).toBe(lines);
    expect(capToolOutput(tool(lines, { name: "bash" })).hidden).toBeGreaterThan(0);
  });
});
