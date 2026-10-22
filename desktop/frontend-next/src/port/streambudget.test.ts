import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LIVE_PANE_STREAMS, StreamBudget } from "./streambudget";
import type { Subscription } from "./port";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function pane() {
  const calls: string[] = [];
  const sub = Object.assign(() => calls.push("close"), { pause: () => calls.push("pause"), resume: () => calls.push("resume") }) as Subscription;
  return { sub, calls, refresh: vi.fn() };
}

const idle = { visible: false, busy: false };

// The browser gives one origin six connections; the tree holds one and a request
// needs one, so at most this many panes may hold a stream at once.
describe("the panes that hold a live stream", () => {
  it("never exceeds the budget, however many panes are open", () => {
    const budget = new StreamBudget();
    const panes = Array.from({ length: 8 }, () => pane());
    panes.forEach((p, i) => budget.attach(`r${i}`, p.sub, p.refresh, i === 0 ? { visible: true, busy: false } : idle));
    expect(budget.liveIds()).toHaveLength(LIVE_PANE_STREAMS);
    expect(budget.liveIds()).toContain("r0");
  });

  it("keeps the visible pane first, then the busy ones, with 8 panes and 4 running", () => {
    const budget = new StreamBudget();
    const panes = Array.from({ length: 8 }, () => pane());
    panes.forEach((p, i) => budget.attach(`r${i}`, p.sub, p.refresh, { visible: i === 7, busy: i >= 3 && i <= 6 }));
    const live = budget.liveIds();
    expect(live).toHaveLength(3);
    expect(live).toContain("r7");
    expect(live.filter((id) => id !== "r7").every((id) => ["r3", "r4", "r5", "r6"].includes(id))).toBe(true);
  });

  it("gives an idle pane's connection back and takes it again when the pane is shown", () => {
    const budget = new StreamBudget();
    const panes = Array.from({ length: 5 }, () => pane());
    panes.forEach((p, i) => budget.attach(`r${i}`, p.sub, p.refresh, { visible: i === 0, busy: false }));
    expect(panes.filter((p) => p.calls.includes("pause"))).toHaveLength(2);
    const paused = panes.findIndex((p) => p.calls.includes("pause"));
    budget.update("r0", idle);
    budget.update(`r${paused}`, { visible: true, busy: false });
    expect(panes[paused].calls).toContain("resume");
    expect(budget.liveIds()).toContain(`r${paused}`);
    expect(budget.liveIds()).toHaveLength(3);
  });

  it("reads a busy pane that lost its stream by polling, and stops when it is no longer owed", () => {
    const budget = new StreamBudget(() => 1);
    const shown = pane();
    const running = pane();
    budget.attach("a", shown.sub, shown.refresh, { visible: true, busy: false });
    budget.attach("b", running.sub, running.refresh, { visible: false, busy: true });
    expect(running.calls).toEqual(["pause"]);
    vi.advanceTimersByTime(5_000);
    expect(running.refresh).toHaveBeenCalledTimes(1);
    expect(shown.refresh).not.toHaveBeenCalled();
    budget.update("b", idle);
    vi.advanceTimersByTime(20_000);
    expect(running.refresh).toHaveBeenCalledTimes(1);
  });

  it("frees a place when a pane goes away", () => {
    const budget = new StreamBudget(() => 1);
    const a = pane();
    const b = pane();
    const releaseA = budget.attach("a", a.sub, a.refresh, { visible: true, busy: false });
    budget.attach("b", b.sub, b.refresh, idle);
    expect(b.calls).toEqual(["pause"]);
    releaseA();
    expect(b.calls).toEqual(["pause", "resume"]);
  });

  it("leaves a transport that cannot pause alone", () => {
    const budget = new StreamBudget(() => 1);
    const bare = Object.assign(() => {}, {}) as Subscription;
    budget.attach("a", bare, () => {}, idle);
    budget.attach("b", bare, () => {}, idle);
    expect(budget.liveIds()).toEqual(["a", "b"]);
  });
});

describe("the number of live streams a person chose", () => {
  it("follows the limit as it is read, giving places out again when it changes", () => {
    let limit = 1;
    const budget = new StreamBudget(() => limit);
    const panes = Array.from({ length: 4 }, () => pane());
    panes.forEach((p, i) => budget.attach(`r${i}`, p.sub, p.refresh, { visible: i === 0, busy: false }));
    expect(budget.liveIds()).toEqual(["r0"]);
    limit = 3;
    budget.resettle();
    expect(budget.liveIds()).toHaveLength(3);
    limit = 1;
    budget.resettle();
    expect(budget.liveIds()).toHaveLength(1);
  });
});
