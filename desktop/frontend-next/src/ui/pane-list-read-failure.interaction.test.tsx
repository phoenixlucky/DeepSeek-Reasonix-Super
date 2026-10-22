// @vitest-environment jsdom
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import "./testkit";
import { App } from "./App";
import { MockHub } from "../port/mock_hub";
import { boot, STORAGE } from "../i18n";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  localStorage.setItem(STORAGE, "zh");
  localStorage.setItem("rx-show-session-tabs", "on");
  boot();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
});

function hubWithTwoPanes() {
  const hub = new MockHub();
  hub.remoteHosts = async () => [];
  const listeners = new Set<() => void>();
  hub.onTreeChanged = (fn) => {
    listeners.add(fn);
    return () => { listeners.delete(fn); };
  };
  const portFor = hub.portFor.bind(hub);
  hub.portFor = (runtime) => {
    const port = portFor(runtime);
    port.providerSetup = async () => null;
    port.welcomeSeen = async () => true;
    return port;
  };
  return { hub, notify: () => listeners.forEach((fn) => fn()) };
}

const tabs = (root: HTMLElement) => Array.from(root.querySelectorAll<HTMLElement>('[role="tab"][data-pane]'));
const selected = (root: HTMLElement) => tabs(root).find((tab) => tab.getAttribute("aria-selected") === "true")?.dataset.pane;

// A read that failed says nothing about the panes: the window has no right to
// conclude they are gone, close every tab and put the person on another one.
it("keeps the open pane when the list of panes cannot be read", async () => {
  const { hub, notify } = hubWithTwoPanes();
  await hub.open({ root: "~/projects/my-website", sessionPath: "/sessions/two.jsonl" });
  const view = render(<App hub={hub} />);
  await waitFor(() => expect(tabs(view.container)).toHaveLength(2), { timeout: 15_000 });
  await act(async () => { tabs(view.container)[1].click(); });
  await waitFor(() => expect(selected(view.container)).toBe("r2"));

  const read = vi.spyOn(hub, "runtimes").mockRejectedValueOnce(new Error("Failed to fetch"));
  await act(async () => { notify(); });
  await waitFor(() => expect(read).toHaveBeenCalled());
  await act(async () => { await Promise.resolve(); });

  expect(tabs(view.container)).toHaveLength(2);
  expect(selected(view.container)).toBe("r2");
});

it("still follows the list when a read succeeds and a pane has really gone", async () => {
  const { hub, notify } = hubWithTwoPanes();
  await hub.open({ root: "~/projects/my-website", sessionPath: "/sessions/two.jsonl" });
  const view = render(<App hub={hub} />);
  await waitFor(() => expect(tabs(view.container)).toHaveLength(2), { timeout: 15_000 });
  await act(async () => { tabs(view.container)[1].click(); });
  await waitFor(() => expect(selected(view.container)).toBe("r2"));

  const first = (await hub.runtimes()).filter((rt) => rt.id === "r1");
  vi.spyOn(hub, "runtimes").mockResolvedValue(first);
  await act(async () => { notify(); });
  await waitFor(() => expect(view.container.querySelectorAll("section.pane")).toHaveLength(1));
  expect(view.container.querySelectorAll("section.pane[data-active]")).toHaveLength(1);
});
