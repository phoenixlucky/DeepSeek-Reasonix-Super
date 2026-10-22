// @vitest-environment jsdom
import { expect, it } from "vitest";
import { MockHub } from "./mock_hub";

it("keeps session placement and pin/archive flags in the same mock tree", async () => {
  const hub = new MockHub();
  const original = (await hub.tree())[0].sessions;
  const [first, second] = original;
  await hub.pinSession(second.path, true);
  await hub.moveSession(second.path, -1);
  const moved = (await hub.tree())[0].sessions;
  expect(moved.map((s) => s.path)).toEqual([second.path, first.path]);
  expect(moved[0].pinned).toBe(true);
  await hub.archiveSession(second.path, true);
  const archived = (await hub.tree())[0].sessions;
  expect(archived.map((s) => s.path)).toEqual([second.path, first.path]);
  expect(archived[0]).toMatchObject({ archived: true, pinned: false });
});
