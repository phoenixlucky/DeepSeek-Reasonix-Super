// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { HttpError } from "../port/port";
import { dismissToast } from "../state/toasts";
import type { UpdateMode } from "../port/version";
import { resetUpdateWatch, startUpdateWatch } from "../state/updatewatch";
import { ToastStack } from "./ToastStack";

afterEach(() => {
  cleanup();
  resetUpdateWatch();
  vi.useRealTimers();
});

// The watch reports a release waiting; the window only shows it.
async function waiting(version = "2.21.0") {
  vi.useFakeTimers();
  const feeds = new Set<(p: never) => void>();
  const stop = startUpdateWatch({
    updateMode: () => Promise.resolve("download" as UpdateMode),
    versions: () => Promise.resolve({ current: "2.20.0", pinned: "", stalePin: false, latest: version, newer: true, versions: [] }),
    goToVersion: () => Promise.resolve(),
    onUpdateProgress: (cb) => { feeds.add(cb as never); queueMicrotask(() => cb({ version, phase: "ready", received: 0, total: 0 })); return () => void feeds.delete(cb as never); },
  });
  await act(async () => { await vi.advanceTimersByTimeAsync(2 * 60_000); });
  vi.useRealTimers();
  return stop;
}

const view = (port: unknown, idle = true) => render(<ToastStack onFeedback={() => {}} port={port as never} idle={idle} />);

describe("the release waiting for a restart", () => {
  it("draws nothing until the watch says a release is downloaded", () => {
    const { container } = view({});
    expect(container.textContent).toBe("");
  });

  it("offers the restart as a notice once nothing is running, and restarts only when asked", async () => {
    const stop = await waiting();
    const restart = vi.fn(() => Promise.resolve());
    view({ restartToVersion: restart });
    expect(await screen.findByText(/新版本 2\.21\.0 已下载/)).toBeTruthy();
    expect(restart).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "重启并更新" }));
    expect(restart).toHaveBeenCalledExactlyOnceWith("2.21.0", false);
    stop();
  });

  it("stays out of the way while a session is running", async () => {
    const stop = await waiting();
    const { container } = view({ restartToVersion: vi.fn() }, false);
    expect(container.textContent).toBe("");
    stop();
  });

  it("never forces a restart over running work: it says so and keeps the notice", async () => {
    const stop = await waiting();
    const restart = vi.fn(() => Promise.reject(new HttpError(409, "busy", { code: "update.restart_busy", params: { n: 2 } }, true)));
    view({ restartToVersion: restart });
    await userEvent.click(await screen.findByRole("button", { name: "重启并更新" }));
    expect(restart).toHaveBeenCalledExactlyOnceWith("2.21.0", false);
    expect(await screen.findByText(/有 2 个会话还在运行/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "重启并更新" })).toBeTruthy();
    stop();
  });

  it("can be put off, and comes back only with a newer release", async () => {
    const stop = await waiting();
    const { container } = view({ restartToVersion: vi.fn() });
    await userEvent.click(await screen.findByRole("button", { name: "稍后" }));
    expect(container.textContent).toBe("");
    stop();
  });

  it("does not disturb the failure notices beside it", async () => {
    const stop = await waiting();
    view({ restartToVersion: vi.fn() });
    expect(await screen.findByRole("region", { name: "通知" })).toBeTruthy();
    dismissToast(-1);
    stop();
  });
});
