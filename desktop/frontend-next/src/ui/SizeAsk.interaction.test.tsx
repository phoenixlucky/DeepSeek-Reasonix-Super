// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";

vi.mock("../port/host", () => ({ host: () => ({ inShell: () => shell.on }) }));
const shell = { on: true };

import { ASKED, SizeAsk } from "./SizeAsk";

const look = { zoom: 1, zoomRange: { min: 0.8, max: 1.6, step: 0.05 } };
const display = (w: number, ratio: number) => {
  Object.defineProperty(window, "devicePixelRatio", { value: ratio, configurable: true });
  Object.defineProperty(window.screen, "width", { value: w, configurable: true });
};

beforeEach(() => {
  shell.on = true;
  localStorage.clear();
  display(2560, 1);
});
afterEach(cleanup);

describe("asking whether the interface is too small", () => {
  it("asks once on a wide display at 100% scaling with the default size", () => {
    render(<SizeAsk look={look} onLook={() => {}} />);
    expect(screen.getByText(/界面是不是偏小/)).toBeTruthy();
  });

  it.each([
    ["a 1080p display", 1920, 1],
    ["a display Windows already scales", 2560, 1.25],
    ["a display Windows scales to 150%", 3840, 1.5],
  ])("does not ask on %s", (_name, width, ratio) => {
    display(width, ratio);
    const { container } = render(<SizeAsk look={look} onLook={() => {}} />);
    expect(container.textContent).toBe("");
  });

  it("never asks someone who has already chosen a size", () => {
    const { container } = render(<SizeAsk look={{ ...look, zoom: 1.25 }} onLook={() => {}} />);
    expect(container.textContent).toBe("");
  });

  it("never asks in a browser tab, where the browser's own zoom is the answer", () => {
    shell.on = false;
    const { container } = render(<SizeAsk look={look} onLook={() => {}} />);
    expect(container.textContent).toBe("");
  });

  it("does not change anything until the person answers", () => {
    const onLook = vi.fn();
    render(<SizeAsk look={look} onLook={onLook} />);
    expect(onLook).not.toHaveBeenCalled();
  });

  it("applies the chosen size at once as a preview and offers to put it back", async () => {
    const onLook = vi.fn();
    const { rerender } = render(<SizeAsk look={look} onLook={onLook} />);
    await userEvent.click(screen.getByRole("button", { name: "125%" }));
    expect(onLook).toHaveBeenCalledExactlyOnceWith({ ...look, zoom: 1.25 });
    rerender(<SizeAsk look={{ ...look, zoom: 1.25 }} onLook={onLook} />);
    await userEvent.click(screen.getByRole("button", { name: "恢复 100%" }));
    expect(onLook).toHaveBeenLastCalledWith({ ...look, zoom: 1 });
  });

  it("leaves the size alone when the person says it is fine, and does not ask again", async () => {
    const onLook = vi.fn();
    const { rerender } = render(<SizeAsk look={look} onLook={onLook} />);
    await userEvent.click(screen.getByRole("button", { name: "不用了" }));
    expect(onLook).not.toHaveBeenCalled();
    expect(localStorage.getItem(ASKED)).toBe("1");
    rerender(<SizeAsk look={look} onLook={onLook} />);
    expect(screen.queryByText(/界面是不是偏小/)).toBeNull();
  });

  it("does not ask again after an earlier answer, even after a restart", () => {
    localStorage.setItem(ASKED, "1");
    const { container } = render(<SizeAsk look={look} onLook={() => {}} />);
    expect(container.textContent).toBe("");
  });

  it("keeps the new size once the person confirms it", async () => {
    const onLook = vi.fn();
    const { rerender } = render(<SizeAsk look={look} onLook={onLook} />);
    await userEvent.click(screen.getByRole("button", { name: "150%" }));
    rerender(<SizeAsk look={{ ...look, zoom: 1.5 }} onLook={onLook} />);
    await userEvent.click(screen.getByRole("button", { name: "好" }));
    expect(screen.queryByRole("button", { name: "恢复 100%" })).toBeNull();
    expect(onLook).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem(ASKED)).toBe("1");
  });
});
