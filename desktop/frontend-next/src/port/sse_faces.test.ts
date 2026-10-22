import { afterEach, expect, it, vi } from "vitest";
import { HttpError, type HookEntry } from "./port";
import { SsePort } from "./sse";

afterEach(() => vi.unstubAllGlobals());

// The real port is a chain of faces. A method an outer face declares again hides
// the inner one, and a fix made in the inner one then never runs.
it("no face of the real port redeclares a method of a face it extends", () => {
  const repeated: string[] = [];
  for (let face = SsePort.prototype; face !== Object.prototype; face = Object.getPrototypeOf(face)) {
    for (const name of Object.getOwnPropertyNames(face)) {
      if (name === "constructor") continue;
      for (let below = Object.getPrototypeOf(face); below !== Object.prototype; below = Object.getPrototypeOf(below)) {
        if (Object.hasOwn(below, name)) repeated.push(`${face.constructor.name}.${name} hides ${below.constructor.name}.${name}`);
      }
    }
  }
  expect(repeated).toEqual([]);
});

function refusing(status: number, refusal: { code: string; error: string }) {
  const body = JSON.stringify(refusal);
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status, text: async () => body, json: async () => refusal })));
}

const hook: HookEntry = { event: "PreToolUse", command: "true", scope: "project" };

it.each([
  ["probeProvider", 401, "provider.probe.unauthorized", (p: SsePort) => p.probeProvider("https://api.example.test", "sk-test")],
  ["dryRunHook", 400, "hooks.dry_run_failed", (p: SsePort) => p.dryRunHook(hook)],
])("%s hands the refusal's code to the reader", async (_, status, code, call) => {
  refusing(status, { code, error: "refused" });
  const err = await call(new SsePort("", "r1")).catch((e: unknown) => e);
  expect(err).toBeInstanceOf(HttpError);
  expect((err as HttpError).reason?.code).toBe(code);
});
