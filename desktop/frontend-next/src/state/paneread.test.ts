import { expect, it } from "vitest";
import { paneReader } from "./paneread";
import type { RuntimeView } from "../port/hub";

const pane = { id: "r1", base: "/rt/r1", root: "/w", name: "w" } as RuntimeView;

it("reads as empty when the very first read fails, so a window is never left blank", async () => {
  const read = paneReader(() => Promise.reject(new Error("offline")));
  expect(await read()).toEqual([]);
});

it("returns null for a failure once a read has answered, and recovers on the next answer", async () => {
  let fail = false;
  const read = paneReader(async () => {
    if (fail) throw new Error("offline");
    return [pane];
  });
  expect(await read()).toEqual([pane]);
  fail = true;
  expect(await read()).toBeNull();
  fail = false;
  expect(await read()).toEqual([pane]);
});
