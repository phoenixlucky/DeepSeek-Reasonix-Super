// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { SseHub, type RuntimeView } from "./hub";
import { RemoteEventSource, remoteCodec, remoteHub } from "./cloud_remote";

const pane = { id: "r1", base: "/rt/r1", root: "/w", name: "w" } as RuntimeView;

function recordSources() {
  const urls: string[] = [];
  const feeds: Array<(raw: string) => void> = [];
  vi.stubGlobal("EventSource", class {
    onmessage: ((m: { data: string }) => void) | null = null;
    constructor(url: string) {
      urls.push(url);
      feeds.push((raw) => this.onmessage?.({ data: raw }));
    }
    close() {}
  });
  return { urls, feeds };
}

// Answers the encrypted handshake the way a desktop does, so remoteHub() can be
// entered without a relay. Requests after it are left unanswered: a test that
// only reads the hub never needs one.
async function reachRemote(deviceId: string) {
  const device = await crypto.subtle.generateKey({ name: "X25519" }, true, ["deriveBits"]);
  const deviceKey = remoteCodec.bytesToBase64(new Uint8Array(await crypto.subtle.exportKey("raw", device.publicKey)));
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    if (!String(input).includes("/me/remote-grants")) throw new Error(`unexpected request ${String(input)}`);
    return {
      ok: true,
      status: 200,
      json: async () => ({
        grant: { ticket: "t" },
        device: { id: deviceId, publicKey: deviceKey, capabilities: ["desktop"] },
      }),
    } as Response;
  });

  const info = new TextEncoder().encode(`reasonix-remote-v1|${deviceId}`);
  class Socket {
    static OPEN = 1;
    readyState = 0;
    private readonly listeners = new Map<string, Set<(e: Event) => void>>();
    constructor(readonly url: string) {
      queueMicrotask(() => {
        this.readyState = 1;
        this.emit("open", new Event("open"));
      });
    }
    addEventListener(type: string, fn: (e: Event) => void) {
      const set = this.listeners.get(type) ?? new Set();
      set.add(fn);
      this.listeners.set(type, set);
    }
    removeEventListener(type: string, fn: (e: Event) => void) {
      this.listeners.get(type)?.delete(fn);
    }
    close() {}
    send(wire: string) {
      void this.answer(wire);
    }
    private emit(type: string, event: Event) {
      for (const fn of this.listeners.get(type) ?? []) fn(event);
    }
    private async answer(wire: string) {
      const hello = JSON.parse(wire) as { type?: string; publicKey?: string; salt?: string };
      if (hello.type !== "hello") return;
      const peer = await crypto.subtle.importKey("raw", remoteCodec.base64ToBytes(hello.publicKey ?? ""), { name: "X25519" }, false, []);
      const secret = new Uint8Array(await crypto.subtle.deriveBits({ name: "X25519", public: peer }, device.privateKey, 256));
      const material = await crypto.subtle.importKey("raw", secret, "HKDF", false, ["deriveKey"]);
      const key = await crypto.subtle.deriveKey(
        { name: "HKDF", hash: "SHA-256", salt: remoteCodec.base64ToBytes(hello.salt ?? ""), info },
        material,
        { name: "AES-GCM", length: 256 },
        false,
        ["encrypt"],
      );
      const nonce = crypto.getRandomValues(new Uint8Array(12));
      const ready = new TextEncoder().encode(JSON.stringify({
        v: 1, type: "ready", deviceId, features: [], session: "s", instance: "i",
      }));
      const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce, additionalData: info }, key, ready);
      this.emit("message", new MessageEvent("message", {
        data: JSON.stringify({
          v: 1,
          nonce: remoteCodec.bytesToBase64(nonce),
          ciphertext: remoteCodec.bytesToBase64(new Uint8Array(ciphertext)),
        }),
      }));
    }
  }
  vi.stubGlobal("WebSocket", Socket);
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the stream a pane reads", () => {
  // A remote hub's EventSource is RemoteEventSource, which polls one replay
  // route. The multiplexed stream also needs an open frame and per-pane PUTs,
  // so it cannot be carried by that transport: the pane reads its own stream.
  it("is the pane's own /events when the hub is reached over the relay", () => {
    const { urls, feeds } = recordSources();
    const seen: unknown[] = [];
    const stop = new SseHub({ mux: false }).portFor(pane).subscribe((ev) => seen.push(ev));

    expect(urls).toEqual(["/rt/r1/events"]);
    feeds[0](JSON.stringify({ kind: "turn_started", seq: 1 }));
    expect(seen).toEqual([{ kind: "turn_started", seq: 1 }]);
    stop();
  });

  it("stays on the hub's one shared stream for a local pane", () => {
    const { urls } = recordSources();
    const stop = new SseHub().portFor(pane).subscribe(() => {});

    expect(urls).toEqual(["/events/mux"]);
    stop();
  });

  // The entry the hosted page actually takes. Without this case, closing mux
  // again in remoteHub would leave every other test here passing.
  it("comes from remoteHub with the multiplexed stream off", async () => {
    await reachRemote("dev1");
    const hub = await remoteHub("dev1");

    expect(globalThis.EventSource).toBe(RemoteEventSource);
    expect((hub.portFor(pane) as unknown as { mux: boolean }).mux).toBe(false);
  });

  // A remote pane reads its own stream, RemoteEventSource turns that into
  // replay polls, and a run's next frame reaches the reducer without a reload.
  it("delivers a new frame through the relay's replay polls", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ frames: [], complete: true, watermark: 0 }), { status: 200 }))
      .mockResolvedValue(new Response(JSON.stringify({
        frames: [{ kind: "turn_started", seq: 1 }],
        complete: true,
        watermark: 1,
      }), { status: 200 }));
    vi.stubGlobal("EventSource", RemoteEventSource);

    const seen: unknown[] = [];
    const stop = new SseHub({ mux: false }).portFor(pane).subscribe((ev) => seen.push(ev));
    await vi.advanceTimersByTimeAsync(0);
    expect(String(fetchMock.mock.calls[0][0])).toBe("/rt/r1/events/replay");

    await vi.advanceTimersByTimeAsync(1500);
    expect(seen).toEqual([{ kind: "turn_started", seq: 1 }]);
    stop();
  });
});
