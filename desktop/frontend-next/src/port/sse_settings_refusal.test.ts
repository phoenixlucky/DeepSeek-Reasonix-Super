import { afterEach, expect, it, vi } from "vitest";
import { HttpError, type NetworkSettings } from "./port";
import { SsePort } from "./sse";

afterEach(() => vi.unstubAllGlobals());

function refusing(status: number, refusal: { code: string; error: string }) {
  const body = JSON.stringify(refusal);
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status, text: async () => body, json: async () => refusal })));
}

const proxy: NetworkSettings = { mode: "custom", effective: "" };

// The settings panels word a refusal from its code; a port that throws only the
// kernel's text leaves them printing that fallback in whatever language it is.
it.each([
  ["saveNetwork", 400, "network.rejected", (p: SsePort) => p.saveNetwork(proxy, "", false)],
  ["uploadWallpaper", 422, "wallpaper.unsupported_type", (p: SsePort) => p.uploadWallpaper(new Blob(["BM"], { type: "image/bmp" }))],
  ["clearWallpaper", 403, "loopback.unauthorized", (p: SsePort) => p.clearWallpaper()],
  ["accountPoll", 403, "account.signin_disabled", (p: SsePort) => p.accountPoll("device-secret")],
])("%s hands the refusal's code to the reader", async (_, status, code, call) => {
  refusing(status, { code, error: "refused" });
  const err = await call(new SsePort("", "r1")).catch((e: unknown) => e);
  expect(err).toBeInstanceOf(HttpError);
  expect((err as HttpError).reason?.code).toBe(code);
});
