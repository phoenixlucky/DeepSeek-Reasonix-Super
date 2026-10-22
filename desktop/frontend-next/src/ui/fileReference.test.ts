import { expect, it } from "vitest";
import { fileReference } from "./fileReference";

it.each([
  ["docs/方案 100%.md", "@docs/方案\\ 100%.md"],
  [String.raw`literal\name.txt`, String.raw`@literal\name.txt`],
  [String.raw`before\ space.txt`, String.raw`@before\\ space.txt`],
  [String.raw`repeat\\ space.txt`, String.raw`@repeat\\\ space.txt`],
  ["before\\\ttab.txt", "@before\\\\\ttab.txt"],
  [String.raw`\\server\share\team notes.txt`, String.raw`@\\server\share\team\ notes.txt`],
])("preserves the kernel @-token spelling for %j", (path, expected) => {
  expect(fileReference(path)).toBe(expected);
});

it.each(["", "trailing\\", "trailing ", "line\nbreak", "line\rbreak", "line\fbreak", "notes)", "notes.md:10", "server:resource", String.raw`C:\notes.md`])(
  "refuses an ambiguous or unrepresentable reference %j", (path) => expect(fileReference(path)).toBe(""),
);
