import { expect, it } from "vitest";

const CSS = import.meta.glob("./studio.css", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const css = CSS["./studio.css"];

function badge(): string {
  const rules = [...css.matchAll(/\.studio-brand-sub\s*\{([^}]*)\}/g)].map((m) => m[1]!);
  expect(rules.length).toBeGreaterThan(0);
  return rules.join("\n");
}

it("draws the Studio badge as an outline, not a filled block", () => {
  const rule = badge();
  expect(rule).toMatch(/background:\s*transparent/);
  expect(rule).toMatch(/border:\s*1px solid var\(--hair\)/);
  expect(rule).toMatch(/color:\s*var\(--muted\)/);
  expect(rule).not.toMatch(/background:\s*var\(--text\)/);
});

it("keeps the badge the same outer size as before the border was added", () => {
  expect(badge()).toMatch(/padding:\s*2px 5px 2px 6px/);
});
