import { describe, expect, it } from "vitest";

const NARROW = Object.values(
  import.meta.glob("./narrow.css", { query: "?raw", import: "default", eager: true }) as Record<string, string>,
)[0];

function declarations(block: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return block.match(new RegExp(String.raw`(?:^|\n)\s*${escaped}\s*\{([^}]*)\}`))?.[1] ?? "";
}

const NARROW_ROW = NARROW.match(/@container agents \(max-width:\s*460px\)\s*\{([\s\S]*?)\n\}/)?.[1] ?? "";

describe("a sub-agent row in a narrow list", () => {
  it("is measured against the list's own width", () => {
    expect(NARROW).toMatch(/\.agents\s*\{\s*container:\s*agents\s*\/\s*inline-size\s*\}/);
    expect(NARROW_ROW, "no narrow rule for the row").not.toBe("");
  });

  it("gives the task a line of its own instead of what the other cells leave", () => {
    const label = declarations(NARROW_ROW, ".ag .lbl");
    expect(label).toMatch(/grid-row:\s*2/);
    expect(label).toMatch(/grid-column:\s*2\s*\/\s*-1/);
  });

  it("keeps the name and the status on the first line and the model chip on the third", () => {
    expect(declarations(NARROW_ROW, ".ag .pip, .ag .nm, .ag .rt, .ag .ag-open")).toMatch(/grid-row:\s*1/);
    expect(declarations(NARROW_ROW, ".ag .ran")).toMatch(/grid-row:\s*3/);
  });
});
