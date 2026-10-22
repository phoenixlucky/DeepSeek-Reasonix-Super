import { expect, it } from "vitest";
import type { Item } from "../../state/session";
import { deliveriesByReply } from "./deliveries";
import { fromHistory, initialState, reduce } from "../../state/session";

it("keeps the latest declaration per file within a turn and preserves previous turns", () => {
  const tool = (id: string, description: string): Item => ({ t: "tool", id, running: false, children: [], tool: {
    name: "present_files", readOnly: true, deliveries: [{ path: "report.md", description }],
  } });
  const items: Item[] = [
    { t: "user", id: "u1", text: "one" },
    tool("t1", "old"), tool("t2", "new"),
    { t: "say", id: "s1", text: "done", done: true },
    { t: "user", id: "u2", text: "two" },
    tool("t3", "second"),
    { t: "say", id: "s2", text: "done", done: true },
  ];
  const map = deliveriesByReply(items);
  expect(map.get("s1")?.map((f) => f.description)).toEqual(["new"]);
  expect(map.get("s2")?.map((f) => f.description)).toEqual(["second"]);
});

it("preserves structured deliveries through live folding and history restoration", () => {
  const files = [{ root: "/granted", path: "report 100%.md", description: "Report" }];
  const tool = { id: "present", name: "present_files", readOnly: true, output: "done", deliveries: files };
  const live = reduce(reduce(initialState, { kind: "tool_dispatch", tool: { id: tool.id, name: tool.name, readOnly: true } }), { kind: "tool_result", tool });
  const restored = fromHistory([
    { role: "assistant", content: "", toolCalls: [{ id: tool.id, name: tool.name }] },
    { role: "tool", content: "done", toolCallId: tool.id, toolName: tool.name, deliveries: files },
  ]);
  expect([...deliveriesByReply(live.items).values()]).toEqual([files]);
  expect([...deliveriesByReply(restored.items).values()]).toEqual([files]);
  const failed = fromHistory([
    { role: "assistant", content: "", toolCalls: [{ id: tool.id, name: tool.name }] },
    { role: "tool", content: "failed", toolCallId: tool.id, toolName: tool.name, toolFailed: true, deliveries: files },
  ]);
  expect(deliveriesByReply(failed.items).size).toBe(0);
});
