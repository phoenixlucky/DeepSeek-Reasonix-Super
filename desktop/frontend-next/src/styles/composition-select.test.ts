import { describe, expect, it } from "vitest";

const CSS = Object.values(
  import.meta.glob("./composition-presets.css", { query: "?raw", import: "default", eager: true }) as Record<string, string>,
)[0];

function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return CSS.match(new RegExp(String.raw`(?:^|\n)${escaped}\s*\{([^}]*)\}`))?.[1] ?? "";
}

describe("the preset editor's drop-downs", () => {
  it("draw their own arrow, set in from the right border, with room for it beside the text", () => {
    const select = rule(".composition-editor select");
    expect(select, "no rule for the editor's selects").not.toBe("");
    expect(select).toMatch(/appearance:\s*none/);
    expect(select).toMatch(/background-position:\s*right\s+14px\s+center/);
    const padding = Number(select.match(/padding-inline-end:\s*(\d+)px/)?.[1] ?? 0);
    expect(padding).toBeGreaterThanOrEqual(36);
  });

  it("leave the add-module button a gap below the list above it", () => {
    const gap = rule(".composition-modules > :is(select, .composition-module) + button");
    expect(Number(gap.match(/margin-top:\s*(\d+)px/)?.[1] ?? 0)).toBeGreaterThanOrEqual(10);
  });
});
