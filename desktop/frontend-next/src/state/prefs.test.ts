// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { liveGraceMinutes, LIVE_GRACE_CHOICES, onLiveGraceChange, onOpenSessionLimitChange, openSessionLimit, setLiveGraceMinutes, setOpenSessionLimit } from "./prefs";

beforeEach(() => localStorage.clear());

describe("how long a finished session stays in the running list", () => {
  it("is off until this machine chooses otherwise", () => {
    expect(liveGraceMinutes()).toBe(0);
  });

  it("reads back a choice from the offered list", () => {
    for (const minutes of LIVE_GRACE_CHOICES) {
      setLiveGraceMinutes(minutes);
      expect(liveGraceMinutes()).toBe(minutes);
    }
  });

  it("treats anything unreadable, unoffered or negative as off", () => {
    for (const raw of ["", "abc", "-5", "7", "1e3", "30 "]) {
      localStorage.setItem("rx-live-grace", raw);
      expect(liveGraceMinutes(), raw).toBe(0);
    }
  });

  it("tells subscribers when the choice changes", () => {
    let heard = 0;
    const stop = onLiveGraceChange(() => heard++);
    setLiveGraceMinutes(30);
    stop();
    setLiveGraceMinutes(60);
    expect(heard).toBe(1);
  });
});

describe("how many sessions stay open as tabs", () => {
  it("never folds one on its own until this machine chooses a number", () => {
    expect(openSessionLimit()).toBe(0);
  });

  it("reads back any non-negative whole number, without an upper bound", () => {
    for (const n of [0, 1, 4, 12, 40, 1000]) {
      setOpenSessionLimit(n);
      expect(openSessionLimit(), String(n)).toBe(n);
    }
  });

  it("treats anything missing, unreadable, fractional or negative as never folding", () => {
    for (const raw of ["", "abc", "-3", "2.5", "1e3", "4 ", " 4", "NaN", "Infinity", "0x10", "9007199254740993"]) {
      localStorage.setItem("rx-open-session-limit", raw);
      expect(openSessionLimit(), raw).toBe(0);
    }
    localStorage.removeItem("rx-open-session-limit");
    expect(openSessionLimit()).toBe(0);
  });

  it("refuses to store a choice that is not a non-negative whole number", () => {
    setOpenSessionLimit(6);
    for (const bad of [-1, 1.5, Number.NaN, Number.POSITIVE_INFINITY]) {
      setOpenSessionLimit(bad);
      expect(openSessionLimit(), String(bad)).toBe(0);
    }
  });

  it("tells subscribers when the choice changes", () => {
    let heard = 0;
    const stop = onOpenSessionLimitChange(() => heard++);
    setOpenSessionLimit(8);
    stop();
    setOpenSessionLimit(4);
    expect(heard).toBe(1);
  });
});
