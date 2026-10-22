import { afterEach, expect, it, vi } from "vitest";
import { SseHttp } from "./sse_http";

class Probe extends SseHttp {
  constructor() {
    super("http://kernel.test");
  }
  read<T>(path: string, signal: AbortSignal) {
    return this.getUntil<T>(path, signal);
  }
}

afterEach(() => vi.restoreAllMocks());

it("passes the caller's signal to the request and never shares it with another read", async () => {
  const signals: (AbortSignal | undefined)[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_url: string, init?: RequestInit) => {
    signals.push(init?.signal ?? undefined);
    return new Response(JSON.stringify({ ok: true }), { status: 200 });
  }));
  const a = new AbortController();
  const b = new AbortController();
  const probe = new Probe();
  await Promise.all([probe.read("/complete?line=/", a.signal), probe.read("/complete?line=/", b.signal)]);
  expect(signals).toEqual([a.signal, b.signal]);
  vi.unstubAllGlobals();
});
