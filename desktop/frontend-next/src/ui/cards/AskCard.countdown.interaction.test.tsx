// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "../testkit";
import { AskCard } from "./AskCard";
import type { Item } from "../../state/session";

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const item = (over: Partial<Extract<Item, { t: "ask" }>> = {}): Extract<Item, { t: "ask" }> => ({
  t: "ask", id: "row", ask: { id: "ask-1", questions: [{ id: "q", prompt: "Pick", options: [{ label: "A" }, { label: "B" }] }] }, ...over,
});

it("counts down to the skip and says how to keep the question", () => {
  render(<AskCard item={item({ skipAt: Date.now() + 90_000 })} onAnswer={vi.fn()} />);
  expect(screen.getByRole("timer").textContent).toContain("1:30");
  act(() => { vi.advanceTimersByTime(31_000); });
  expect(screen.getByRole("timer").textContent).toContain("59 秒");
  expect(screen.getByRole("timer").textContent).toContain("点任意处保留");
});

it("shows nothing for a question that is never skipped", () => {
  render(<AskCard item={item()} onAnswer={vi.fn()} />);
  expect(screen.queryByRole("timer")).toBeNull();
});

it("hides the countdown once the person has taken the question over", async () => {
  const onEngage = vi.fn(async () => {});
  render(<AskCard item={item({ skipAt: Date.now() + 60_000 })} onAnswer={vi.fn()} onEngage={onEngage} />);
  expect(screen.getByRole("timer")).toBeTruthy();
  await userEvent.click(screen.getByRole("button", { name: "A" }));
  expect(screen.queryByRole("timer")).toBeNull();
});

it("shows nothing on a card that is already answered", () => {
  render(<AskCard item={item({ skipAt: Date.now() + 60_000, answered: [["A"]] })} onAnswer={vi.fn()} />);
  expect(screen.queryByRole("timer")).toBeNull();
});
