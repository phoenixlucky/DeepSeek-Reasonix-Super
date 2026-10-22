// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import "./testkit";
import { RunLine } from "./RunLine";

afterEach(cleanup);

const draw = (over: Partial<Parameters<typeof RunLine>[0]> = {}) =>
  render(<RunLine label="运行中" running blocked={false} since={Date.now() - 12_000} sent={10} received={20} estimated={false} {...over} />);

describe("the run state beside the composer", () => {
  it("is announced to a screen reader and not printed as a word", () => {
    draw();
    const status = screen.getByRole("status");
    expect(status.getAttribute("aria-live")).toBe("polite");
    const word = screen.getByText("运行中");
    expect(status.contains(word)).toBe(true);
    expect(word.classList.contains("sr-only")).toBe(true);
  });

  it("keeps the clock and the mark where the word used to be", () => {
    draw();
    expect(document.querySelector(".studio-runclock")).not.toBeNull();
    expect(document.querySelector(".rmark")).not.toBeNull();
  });

  it("says idle and waiting the same way, and the state attributes still follow the run", () => {
    const idle = draw({ label: "空闲", running: false, since: undefined });
    expect(screen.getByText("空闲").classList.contains("sr-only")).toBe(true);
    expect(document.querySelector(".studio-runstate[data-idle]")).not.toBeNull();
    idle.unmount();
    draw({ label: "等待你的决定", running: true, blocked: true });
    expect(screen.getByText("等待你的决定").classList.contains("sr-only")).toBe(true);
    expect(document.querySelector(".studio-runstate[data-waiting]")).not.toBeNull();
  });
});
