// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SseHub, type RuntimeView } from "./hub";
import type { WireEvent } from "./wire";

// Every way a hub can be built has to put a frame the kernel sends into the
// pane's reducer. A change to the event transport is held to this table, which
// is where "the page stays connected and goes silent" gets caught.
const rt = { id: "r1", base: "/rt/r1", root: "/w", name: "w" } as RuntimeView;

interface Source { url: string; closed: boolean; say: (data: object) => void }

function kernel(opts: { muxAnswers: boolean }) {
  const sources: Source[] = [];
  const puts: string[] = [];
  vi.stubGlobal(
    "EventSource",
    class {
      onmessage: ((m: { data: string }) => void) | null = null;
      onopen: (() => void) | null = null;
      readonly s: Source;
      constructor(url: string) {
        this.s = { url, closed: false, say: (data) => this.onmessage?.({ data: JSON.stringify(data) }) };
        sources.push(this.s);
        const answers = url.endsWith("/events/mux") ? opts.muxAnswers : true;
        // A kernel that answers the mux path with the SPA shell opens a stream
        // that never says anything, which is what an older kernel does.
        if (answers && url.endsWith("/events/mux")) queueMicrotask(() => this.s.say({ kind: "mux_open", conn: "c1" }));
      }
      close() {
        this.s.closed = true;
      }
    },
  );
  vi.stubGlobal("fetch", async (url: string, init: RequestInit = {}) => {
    if (init.method === "PUT") puts.push(url);
    return { ok: true, status: 204, json: async () => ({}) } as Response;
  });
  return {
    sources,
    puts,
    live: (suffix: string) => sources.filter((s) => !s.closed && s.url.includes(suffix)),
  };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const tick = (ms = 0) => vi.advanceTimersByTimeAsync(ms);

describe("a pane receives what its kernel sends", () => {
  it("on a local hub, through the shared stream", async () => {
    const k = kernel({ muxAnswers: true });
    const got: WireEvent[] = [];
    const stop = new SseHub().portFor(rt).subscribe((ev) => got.push(ev));
    await tick();
    expect(k.puts).toEqual(["/events/mux/c1/panes/r1"]);
    k.live("/events/mux")[0].say({ pane: "r1", frame: { kind: "text", seq: 1, text: "hi" } });
    expect(got.map((e) => e.kind)).toEqual(["text"]);
    expect(k.live("/rt/r1/events")).toHaveLength(0);
    stop();
  });

  it("on a hub reached over a relay, through the pane's own stream and never the shared one", async () => {
    const k = kernel({ muxAnswers: true });
    const got: WireEvent[] = [];
    const stop = new SseHub({ mux: false }).portFor(rt).subscribe((ev) => got.push(ev));
    await tick(20_000);
    expect(k.sources.some((s) => s.url.endsWith("/events/mux"))).toBe(false);
    k.live("/rt/r1/events")[0].say({ kind: "text", seq: 1, text: "hi" });
    expect(got.map((e) => e.kind)).toEqual(["text"]);
    stop();
  });

  it("on a kernel that answers the shared path with something else, through the pane's own stream", async () => {
    const k = kernel({ muxAnswers: false });
    const got: WireEvent[] = [];
    const stop = new SseHub().portFor(rt).subscribe((ev) => got.push(ev));
    await tick(11_000);
    expect(k.live("/events/mux")).toHaveLength(0);
    k.live("/rt/r1/events")[0].say({ kind: "text", seq: 1, text: "hi" });
    expect(got.map((e) => e.kind)).toEqual(["text"]);
    stop();
  });

  it("on every one of them, a second frame follows the first in order", async () => {
    for (const [hub, muxAnswers] of [[new SseHub(), true], [new SseHub({ mux: false }), true], [new SseHub(), false]] as const) {
      const k = kernel({ muxAnswers });
      const got: number[] = [];
      const stop = hub.portFor({ ...rt, id: `r${got.length}` } as RuntimeView).subscribe((ev) => got.push(ev.seq ?? 0));
      await tick(11_000);
      const source = k.live("/events/mux")[0] ?? k.live("/rt/")[0];
      const say = (seq: number) => source.say(source.url.endsWith("/events/mux") ? { pane: "r0", frame: { kind: "tick", seq } } : { kind: "tick", seq });
      say(1);
      say(2);
      expect(got).toEqual([1, 2]);
      stop();
      vi.unstubAllGlobals();
    }
  });
});
