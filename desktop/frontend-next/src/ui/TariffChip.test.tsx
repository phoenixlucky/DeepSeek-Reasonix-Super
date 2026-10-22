// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import "./testkit";
import { TariffChip, until } from "./TariffChip";
import { boot, STORAGE } from "../i18n";

const NOW = Date.parse("2026-10-12T02:00:00Z");

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  localStorage.setItem(STORAGE, "zh");
  boot();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it("names the coarsest true unit and never rounds a minute away", () => {
  expect(until("2026-10-12T02:00:20Z", NOW)).toBe("1 分钟");
  expect(until("2026-10-12T02:45:00Z", NOW)).toBe("45 分钟");
  expect(until("2026-10-12T04:10:00Z", NOW)).toBe("2 小时 10 分钟");
  expect(until("2026-10-12T05:00:00Z", NOW)).toBe("3 小时");
  expect(until("2026-10-14T02:00:00Z", NOW)).toBe("2 天");
  expect(until("2026-10-12T01:59:00Z", NOW)).toBe("");
});

it("says which rate applies and how long until it changes", () => {
  render(<TariffChip tariff={{ peak: true, changesAt: "2026-10-12T04:10:00Z" }} onFlip={() => {}} />);
  const chip = screen.getByText("高峰").parentElement!;
  expect(chip.hasAttribute("data-peak")).toBe(true);
  expect(chip.textContent).toContain("2 小时 10 分钟");
  expect(chip.getAttribute("title")).toBe("现在按供应商公布的高峰价计费，约 2 小时 10 分钟 后恢复平峰价");
});

it("draws the off-peak window without the peak tone", () => {
  render(<TariffChip tariff={{ peak: false, changesAt: "2026-10-12T02:30:00Z" }} onFlip={() => {}} />);
  const chip = screen.getByText("平峰").parentElement!;
  expect(chip.hasAttribute("data-peak")).toBe(false);
  expect(chip.getAttribute("title")).toContain("进入高峰");
});

it("says nothing about a flip when none is ahead", () => {
  render(<TariffChip tariff={{ peak: false }} onFlip={() => {}} />);
  expect(screen.getByText("平峰").parentElement!.querySelector("i")).toBeNull();
});

it("counts down, then asks for a fresh answer exactly once and draws nothing stale", () => {
  const onFlip = vi.fn();
  const { container } = render(<TariffChip tariff={{ peak: true, changesAt: "2026-10-12T02:01:00Z" }} onFlip={onFlip} />);
  expect(screen.getByText("高峰")).toBeTruthy();
  act(() => { vi.advanceTimersByTime(61_000); });
  expect(onFlip).toHaveBeenCalledTimes(1);
  expect(container.textContent).toBe("");
  act(() => { vi.advanceTimersByTime(120_000); });
  expect(onFlip).toHaveBeenCalledTimes(1);
});
