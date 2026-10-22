// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { App } from "./App";
import { MockHub } from "../port/mock_hub";
import { boot, STORAGE } from "../i18n";
import type { Checkpoint } from "../port/port";

// The transport is a fixture; App, Pane, reply actions and SayCard are real.
afterEach(() => { cleanup(); sessionStorage.clear(); localStorage.clear(); vi.restoreAllMocks(); });

it("opens a sidebar conversation through the real final-reply BranchAt action", async () => {
  localStorage.setItem(STORAGE, "zh"); boot();
  const hub = new MockHub();
  const source = (await hub.runtimes())[0];
  const checkpoints: Checkpoint[] = [
    { turn: 0, msgIndex: 1, prompt: "first", files: 0 },
    { turn: 1, msgIndex: 4, prompt: "second", files: 0 },
  ];
  const history = [
    { role: "system" as const, content: "sys", msgIndex: 0 },
    { role: "user" as const, content: "first", msgIndex: 1 },
    { role: "assistant" as const, content: "intermediate reply", msgIndex: 2 },
    { role: "assistant" as const, content: "final first reply", msgIndex: 3 },
    { role: "user" as const, content: "second", msgIndex: 4 },
    { role: "assistant" as const, content: "final second reply", msgIndex: 5 },
  ];
  const portFor = hub.portFor.bind(hub);
  hub.portFor = (rt) => {
    const port = portFor(rt);
    port.providerSetup = async () => null;
    port.history = async () => rt.id === source.id ? history : history.slice(0, 4);
    port.checkpoints = async () => rt.id === source.id ? checkpoints : [{ ...checkpoints[0], forkCopied: true }];
    const status = port.status.bind(port);
    port.status = async () => ({ ...await status(), sessionPath: rt.sessionPath });
    return port;
  };
  vi.spyOn(hub, "tree").mockImplementation(async () => [{
    root: source.root, name: source.name, remembered: true,
    sessions: (await hub.runtimes()).map((rt) => ({ path: rt.sessionPath!, name: rt.id,
      title: rt.id === source.id ? "source conversation" : "child conversation", runtimeId: rt.id, turns: rt.id === source.id ? 2 : 1 })),
  }]);
  let finish: (path: string) => void = () => {};
  const branch = vi.spyOn(hub.portFor(source), "branchAt").mockImplementation(() => new Promise<string>((resolve) => { finish = resolve; }));
  const originalOpen = hub.open.bind(hub);
  let finishOpen = async () => {};
  const open = vi.spyOn(hub, "open").mockImplementation((req) => new Promise((resolve) => {
    finishOpen = async () => { resolve(await originalOpen(req)); };
  }));
  const rename = vi.spyOn(hub, "renameSession");
  const view = render(<App hub={hub} />);
  const active = () => view.container.querySelector<HTMLElement>(".pane[data-active]")!;
  await waitFor(() => expect(active()).toBeTruthy());
  const buttons = await within(active()).findAllByRole("button", { name: "创建对话分支" });
  expect(buttons).toHaveLength(3);
  const intermediate = screen.getByText("intermediate reply").closest('[data-k="say"]')!;
  expect(intermediate.querySelector('[data-action="reply.branch"]')).toBeTruthy();
  await userEvent.click(buttons[0]);
  await userEvent.click(buttons[0]);
  expect(branch).toHaveBeenCalledExactlyOnceWith(0, "source conversation · 分支");
  expect(buttons[0].hasAttribute("disabled")).toBe(true);
  expect(await hub.runtimes()).toHaveLength(1);
  expect(screen.queryByText("child conversation")).toBeNull();
  await act(async () => finish("/sessions/child.jsonl"));
  await waitFor(() => expect(open).toHaveBeenCalledWith({ root: source.root, sessionPath: "/sessions/child.jsonl" }));
  expect(buttons[0].hasAttribute("disabled")).toBe(true);
  await act(finishOpen);
  await waitFor(() => expect(view.container.querySelector(".crumb b")?.textContent).toBe("child conversation"));
  expect(view.container.querySelectorAll(".pane")).toHaveLength(2);
  const row = [...view.container.querySelectorAll(".sessrow")].find((el) => el.textContent?.includes("child conversation"));
  expect(row).toBeTruthy();
  expect(within(active()).queryByText("final second reply")).toBeNull();
  expect(rename).not.toHaveBeenCalled();
});
