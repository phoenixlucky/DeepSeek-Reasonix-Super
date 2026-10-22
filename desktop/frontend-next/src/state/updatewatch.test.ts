import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UpdateMode, VersionHub } from "../port/version";
import { readUpdateAvailable, resetUpdateWatch, startUpdateWatch } from "./updatewatch";

const hub = (over: Partial<VersionHub> = {}): VersionHub => ({
  current: "2.20.0", pinned: "", stalePin: false, latest: "2.21.0", newer: true, versions: [], ...over,
});

function port(mode: UpdateMode, read: () => Promise<VersionHub> = () => Promise.resolve(hub())) {
  return { updateMode: vi.fn(() => Promise.resolve(mode)), versions: vi.fn(read) };
}

const MINUTE = 60_000;
const DAY = 24 * 60 * MINUTE;

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  resetUpdateWatch();
  vi.useRealTimers();
});

describe("the background update check", () => {
  it("reads nothing from the catalog in manual mode", async () => {
    const p = port("manual");
    const stop = startUpdateWatch(p);
    await vi.advanceTimersByTimeAsync(2 * DAY);
    expect(p.versions).not.toHaveBeenCalled();
    expect(readUpdateAvailable()).toBe("");
    stop();
  });

  it("says a newer release exists once the first delay has passed, in notify mode", async () => {
    const p = port("notify");
    const stop = startUpdateWatch(p);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(p.versions).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(40_000);
    expect(p.versions).toHaveBeenCalledTimes(1);
    expect(readUpdateAvailable()).toBe("2.21.0");
    stop();
  });

  it("stays quiet for a pinned build, a withdrawn one, and a catalog with nothing newer", async () => {
    for (const over of [{ pinned: "2.20.0" }, { withdrawn: { version: "2.20.0", reason: "x" } }, { newer: false, latest: "2.20.0" }]) {
      const p = port("notify", () => Promise.resolve(hub(over)));
      const stop = startUpdateWatch(p);
      await vi.advanceTimersByTimeAsync(2 * MINUTE);
      expect(p.versions).toHaveBeenCalled();
      expect(readUpdateAvailable()).toBe("");
      stop();
      resetUpdateWatch();
    }
  });

  it("fails silently and backs off instead of retrying in a storm", async () => {
    const p = port("notify", () => Promise.reject(new Error("offline")));
    const stop = startUpdateWatch(p);
    await vi.advanceTimersByTimeAsync(70_000);
    expect(p.versions).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(14 * MINUTE);
    expect(p.versions).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2 * MINUTE);
    expect(p.versions).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(27 * MINUTE);
    expect(p.versions).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(4 * MINUTE);
    expect(p.versions).toHaveBeenCalledTimes(3);
    expect(readUpdateAvailable()).toBe("");
    stop();
  });

  it("checks again a day after a good answer, and not before", async () => {
    const p = port("notify");
    const stop = startUpdateWatch(p);
    await vi.advanceTimersByTimeAsync(70_000);
    await vi.advanceTimersByTimeAsync(DAY - 10 * MINUTE);
    expect(p.versions).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(11 * MINUTE);
    expect(p.versions).toHaveBeenCalledTimes(2);
    stop();
  });

  it("stops asking, and drops what it said, once the user goes back to manual", async () => {
    let mode: UpdateMode = "notify";
    const p = { updateMode: vi.fn(() => Promise.resolve(mode)), versions: vi.fn(() => Promise.resolve(hub())) };
    const stop = startUpdateWatch(p);
    await vi.advanceTimersByTimeAsync(70_000);
    expect(readUpdateAvailable()).toBe("2.21.0");
    mode = "manual";
    await stop.refresh();
    expect(readUpdateAvailable()).toBe("");
    await vi.advanceTimersByTimeAsync(2 * DAY);
    expect(p.versions).toHaveBeenCalledTimes(1);
    stop();
  });

  it("a refresh after choosing notify checks at once", async () => {
    let mode: UpdateMode = "manual";
    const p = { updateMode: vi.fn(() => Promise.resolve(mode)), versions: vi.fn(() => Promise.resolve(hub())) };
    const stop = startUpdateWatch(p);
    await vi.advanceTimersByTimeAsync(70_000);
    expect(p.versions).not.toHaveBeenCalled();
    mode = "notify";
    await stop.refresh();
    expect(p.versions).toHaveBeenCalledTimes(1);
    expect(readUpdateAvailable()).toBe("2.21.0");
    stop();
  });
});
