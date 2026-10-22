import { afterEach, describe, expect, it, vi } from "vitest";
import { SsePort } from "./sse";

afterEach(() => vi.unstubAllGlobals());

function answers(body: unknown) {
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, status: 200, json: async () => body, text: async () => JSON.stringify(body) })));
}

// A Go nil slice encodes to null and an omitted field is absent; both reach a
// reader that calls .filter on the list and fail with "reading 'filter'".
describe.each([{ label: "an empty object", body: {} }, { label: "null lists", body: { surfaces: null, servers: null, changes: null } }])(
  "a list read answered with $label",
  ({ body }) => {
    it("gives the extension surfaces an empty list", async () => {
      answers(body);
      expect((await new SsePort("", "r1").extensionSurfaces()).surfaces).toEqual([]);
    });
    it("gives the MCP catalog an empty server list", async () => {
      answers(body);
      expect((await new SsePort("", "r1").mcp()).servers).toEqual([]);
    });
    it("gives the changed files an empty list", async () => {
      answers(body);
      expect((await new SsePort("", "r1").changes()).changes).toEqual([]);
    });
  },
);
