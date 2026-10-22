// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { setOpenSessionLimit } from "../state/prefs";
import type { RuntimeView } from "../port/hub";
import type { AgentPort } from "../port/port";
import { useOpenPane } from "./useOpenPane";

beforeEach(() => localStorage.clear());

const view = (n: number, root = "/w"): RuntimeView => ({ id: `r${n}`, root, name: "w", sessionPath: `${root}/s${n}.jsonl` }) as RuntimeView;

// A window with `open` idle sessions in one folder, the first of them in front.
function window(open: number) {
  const runtimes = Array.from({ length: open }, (_, i) => view(i));
  const resume = vi.fn(async () => {});
  const panePorts = new Map(runtimes.map((rt) => [rt.id, { resume } as unknown as AgentPort]));
  let next = open;
  const hub = {
    open: vi.fn(async (req: { root?: string; sessionPath?: string }) => ({ id: `r${next++}`, root: req.root ?? "/w", name: "w", sessionPath: req.sessionPath }) as RuntimeView),
    close: vi.fn(async () => {}),
  };
  const sw = { path: "", pending: false, opening: () => false, begin: () => 1, current: () => true, landed: () => {}, end: () => {}, cancel: () => {} };
  const hook = renderHook(() =>
    useOpenPane({
      hub: hub as never, runtimes, panePorts, active: "r0",
      reportsRef: { current: {} }, runsRef: { current: {} },
      setRuntimes: vi.fn(), setActive: vi.fn(), setTakeover: vi.fn(), reloadPanes: async () => {}, sw,
    }),
  );
  const openMore = (path: string, root = "/w") => act(async () => void (await hook.result.current({ root, sessionPath: path })));
  return { hub, resume, openMore };
}

describe("opening one more session than the window holds", () => {
  it("keeps every session open by default: a new pane each time, none reused, none closed", async () => {
    for (const open of [4, 5, 9]) {
      const w = window(open);
      await w.openMore("/w/new.jsonl");
      expect(w.hub.open, `${open} open`).toHaveBeenCalledTimes(1);
      expect(w.resume, `${open} open`).not.toHaveBeenCalled();
      expect(w.hub.close, `${open} open`).not.toHaveBeenCalled();
    }
  });

  it("folds the oldest idle session away only once the person chose a number and the window reached it", async () => {
    setOpenSessionLimit(4);
    const below = window(3);
    await below.openMore("/w/new.jsonl");
    expect(below.resume).not.toHaveBeenCalled();
    expect(below.hub.open).toHaveBeenCalledTimes(1);

    const full = window(4);
    await full.openMore("/w/new.jsonl");
    expect(full.resume).toHaveBeenCalledWith("/w/new.jsonl");
    expect(full.hub.open).not.toHaveBeenCalled();
  });

  it("closes an idle pane of another folder at the chosen number instead of reusing it", async () => {
    setOpenSessionLimit(8);
    const w = window(8);
    await w.openMore("/other/new.jsonl", "/other");
    expect(w.hub.close).toHaveBeenCalledWith("r1");
    expect(w.hub.open).toHaveBeenCalledTimes(1);
  });

  it("reads the choice at the moment of opening, so changing it needs no restart", async () => {
    const w = window(5);
    setOpenSessionLimit(5);
    await w.openMore("/w/a.jsonl");
    expect(w.resume).toHaveBeenCalledTimes(1);
    setOpenSessionLimit(0);
    await w.openMore("/w/b.jsonl");
    expect(w.resume).toHaveBeenCalledTimes(1);
    expect(w.hub.open).toHaveBeenCalledTimes(1);
  });

  it("treats a corrupt stored choice as never folding", async () => {
    localStorage.setItem("rx-open-session-limit", "four");
    const w = window(6);
    await w.openMore("/w/new.jsonl");
    expect(w.hub.open).toHaveBeenCalledTimes(1);
    expect(w.hub.close).not.toHaveBeenCalled();
  });
});
