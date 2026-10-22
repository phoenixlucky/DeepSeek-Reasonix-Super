// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StopCall } from "./StopCall";

describe("StopCall", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("stays out of the way of a quick call, then offers to stop that one call", () => {
    const onStop = vi.fn();
    render(<StopCall onStop={onStop} />);
    act(() => void vi.advanceTimersByTime(5_000));
    expect(document.querySelector("[data-action='tool.stop']")).toBeNull();

    act(() => void vi.advanceTimersByTime(6_000));
    fireEvent.click(document.querySelector("[data-action='tool.stop']")!);
    expect(onStop).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button")).not.toBeNull();
  });
});
