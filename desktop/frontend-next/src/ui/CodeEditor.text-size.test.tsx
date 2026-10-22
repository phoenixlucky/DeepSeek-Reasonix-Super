// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import CodeEditor from "./CodeEditor";

afterEach(cleanup);

const sizes = () => Array.from(document.querySelectorAll("style")).flatMap((s) => (s.textContent ?? "").match(/font-size:[^;}]*/g) ?? []);

it("sizes its text, and its search field, with the interface text size", () => {
  render(<CodeEditor path="a.ts" value="const a = 1;" dark={false} onChange={() => {}} onSave={() => {}} />);
  const found = sizes();
  expect(found.filter((s) => s.includes("--fs-scale")).length).toBeGreaterThan(0);
  expect(found.filter((s) => s === "font-size: 12px" || s === "font-size: 11px")).toEqual([]);
});
