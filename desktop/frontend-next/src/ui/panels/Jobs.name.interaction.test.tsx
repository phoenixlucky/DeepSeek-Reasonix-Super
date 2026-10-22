// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import "../testkit";
import { Jobs } from "./Jobs";
import type { JobEntry } from "../../port/port";

afterEach(() => { cleanup(); vi.useRealTimers(); });

const job = (over: Partial<JobEntry>): JobEntry => ({ id: "bash-1", kind: "bash", label: "go test ./internal/very/long/pa…", status: "running", startedAt: Date.now(), ...over });
const command = "go test ./internal/very/long/package/path/... -run TestSomething -count=1";

describe("a job in the list", () => {
  it("is called by the name its caller gave it, not by the command", () => {
    render(<Jobs jobs={[job({ description: "Run the package tests", detail: command })]} onView={() => {}} />);
    expect(screen.getByRole("button", { name: /Run the package tests|运行|Run/ }).textContent).toBe("Run the package tests");
    expect(document.body.textContent).not.toContain("go test");
  });

  it("falls back to the preview, then the id, and never draws an empty row", () => {
    render(<Jobs jobs={[job({}), job({ id: "bash-2", label: "" })]} />);
    const names = [...document.querySelectorAll(".job .cmd")].map((e) => e.textContent);
    expect(names).toEqual(["go test ./internal/very/long/pa…", "bash-2"]);
  });

  it("shows the whole name and the whole command on hover, selectable as text", () => {
    render(<Jobs jobs={[job({ description: "Run the package tests", detail: command })]} onView={() => {}} />);
    expect(document.querySelector(".job-tip")).toBeNull();
    fireEvent.mouseEnter(document.querySelector(".job-name")!);
    const tip = document.querySelector(".job-tip")!;
    expect(tip.getAttribute("role")).toBe("tooltip");
    expect(tip.querySelector(".jt-name")?.textContent).toBe("Run the package tests");
    expect(tip.querySelector(".jt-cmd")?.textContent).toBe(command);
    fireEvent.mouseLeave(document.querySelector(".job-name")!);
    expect(document.querySelector(".job-tip")).toBeNull();
  });

  it("is reachable by keyboard: focus shows it, Escape closes it", () => {
    render(<Jobs jobs={[job({ description: "Run the package tests", detail: command })]} onView={() => {}} />);
    const button = document.querySelector<HTMLButtonElement>(".job .cmd")!;
    fireEvent.focus(button);
    expect(document.querySelector(".job-tip .jt-cmd")?.textContent).toBe(command);
    expect(document.querySelector(".job-name")?.getAttribute("aria-describedby")).toBe(document.querySelector(".job-tip")?.id);
    fireEvent.keyDown(button, { key: "Escape" });
    expect(document.querySelector(".job-tip")).toBeNull();
  });

  it("opens on a long press on a touch screen, not on a tap", () => {
    vi.useFakeTimers();
    render(<Jobs jobs={[job({ description: "Run the package tests", detail: command })]} onView={() => {}} />);
    const box = document.querySelector(".job-name")!;
    fireEvent.pointerDown(box, { pointerType: "touch" });
    act(() => { vi.advanceTimersByTime(100); });
    fireEvent.pointerUp(box, { pointerType: "touch" });
    act(() => { vi.advanceTimersByTime(1000); });
    expect(document.querySelector(".job-tip")).toBeNull();
    fireEvent.pointerDown(box, { pointerType: "touch" });
    act(() => { vi.advanceTimersByTime(600); });
    expect(document.querySelector(".job-tip")).not.toBeNull();
  });

  it("gives a job with only a long label its full text, and no second line", () => {
    render(<Jobs jobs={[job({ kind: "task", label: "Audit the whole session store for stale entries" })]} />);
    fireEvent.mouseEnter(document.querySelector(".job-name")!);
    expect(document.querySelector(".jt-name")?.textContent).toBe("Audit the whole session store for stale entries");
    expect(document.querySelector(".jt-cmd")).toBeNull();
  });
});
