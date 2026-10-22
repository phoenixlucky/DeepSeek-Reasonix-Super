// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { LiveStreamBudgetSection } from "./LiveStreamBudgetSection";
import { liveStreamBudget, setLiveStreamBudget } from "../state/prefs";

beforeEach(() => localStorage.clear());
afterEach(cleanup);

const box = () => screen.getByRole("textbox", { name: "实时连接的标签数" }) as HTMLInputElement;

it("starts empty, which is automatic, and nothing is stored", () => {
  render(<LiveStreamBudgetSection />);
  expect(box().value).toBe("");
  expect(liveStreamBudget()).toBeNull();
  expect(localStorage.getItem("rx-live-stream-budget")).toBeNull();
});

it("saves a number from one to four once it is committed", async () => {
  render(<LiveStreamBudgetSection />);
  await userEvent.type(box(), "2{Enter}");
  expect(liveStreamBudget()).toBe(2);
  expect(localStorage.getItem("rx-live-stream-budget")).toBe("2");
});

it("reads a number outside the range as automatic and shows that", async () => {
  render(<LiveStreamBudgetSection />);
  await userEvent.type(box(), "9{Enter}");
  expect(liveStreamBudget()).toBeNull();
  expect(box().value).toBe("");
  expect(localStorage.getItem("rx-live-stream-budget")).toBeNull();
});

it("returns to automatic when the box is emptied, and shows a choice made in another window", async () => {
  setLiveStreamBudget(3);
  render(<LiveStreamBudgetSection />);
  expect(box().value).toBe("3");
  await userEvent.clear(box());
  await userEvent.tab();
  expect(liveStreamBudget()).toBeNull();
  cleanup();
  localStorage.setItem("rx-live-stream-budget", "4");
  render(<LiveStreamBudgetSection />);
  expect(box().value).toBe("4");
});
