// @vitest-environment jsdom
import { useState } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { HubPort, RuntimeView } from "../port/hub";
import { useSessionSwitch } from "./sessionswitch";
import { useOpenRemotePane } from "./useOpenRemotePane";

afterEach(cleanup);

function draw(openRemote = vi.fn<HubPort["openRemote"]>()) {
  const setActive = vi.fn();
  const reloadPanes = vi.fn(async () => {});
  const reloadRemotes = vi.fn(async () => {});
  const hook = renderHook(() => {
    const sw = useSessionSwitch();
    const [opening, setOpening] = useState(0);
    const open = useOpenRemotePane({ hub: { openRemote } as unknown as HubPort, reloadPanes, reloadRemotes, sw, setActive, setOpening });
    return { open, opening, pending: sw.pending };
  });
  return { ...hook, openRemote, setActive, reloadPanes, reloadRemotes };
}

const view = (id: string): RuntimeView => ({ id, base: "/runtime/" + id, host: "gpu", root: "/project", name: id });

it("sends the selected composition to the remote host for each new conversation", async () => {
  const { result, openRemote, setActive } = draw();
  openRemote.mockResolvedValueOnce(view("minimal")).mockResolvedValueOnce(view("standard"));
  await act(async () => { await result.current.open("gpu", "/project", undefined, "builtin:minimal"); });
  expect(openRemote).toHaveBeenLastCalledWith({ host: "gpu", workspace: "/project", sessionPath: undefined, agentComposition: "builtin:minimal" });
  await act(async () => { await result.current.open("gpu", "/project", undefined, "builtin:general"); });
  expect(openRemote).toHaveBeenLastCalledWith({ host: "gpu", workspace: "/project", sessionPath: undefined, agentComposition: "builtin:general" });
  expect(setActive.mock.calls).toEqual([["minimal"], ["standard"]]);
  expect(result.current.opening).toBe(0);
});

it("keeps the newest selection focused when an older open finishes last", async () => {
  const { result, openRemote, setActive } = draw();
  let finish!: (value: RuntimeView) => void;
  openRemote.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; })).mockResolvedValueOnce(view("newer"));
  let older!: Promise<void>;
  act(() => { older = result.current.open("gpu", "/project", undefined, "builtin:minimal"); });
  await act(async () => { await result.current.open("gpu", "/project", undefined, "builtin:general"); });
  await act(async () => { finish(view("older")); await older; });
  expect(setActive.mock.calls).toEqual([["newer"]]);
  expect(result.current.opening).toBe(0);
  expect(result.current.pending).toBe(false);
});

it("coalesces a repeated history open and releases the selection after refusal", async () => {
  const { result, openRemote, reloadRemotes, setActive } = draw();
  let refuse!: (error: Error) => void;
  openRemote.mockImplementationOnce(() => new Promise((_resolve, reject) => { refuse = reject; }));
  let first!: Promise<void>;
  act(() => { first = result.current.open("gpu", "/project", "/project/task.jsonl"); });
  const rejected = expect(first).rejects.toThrow("fixture refusal");
  await act(async () => { await result.current.open("gpu", "/project", "/project/task.jsonl"); });
  expect(openRemote).toHaveBeenCalledOnce();
  await act(async () => { refuse(new Error("fixture refusal")); await rejected; });
  expect(result.current.opening).toBe(0);
  expect(result.current.pending).toBe(false);
  expect(reloadRemotes).toHaveBeenCalledOnce();
  expect(setActive).not.toHaveBeenCalled();
  openRemote.mockResolvedValueOnce(view("retry"));
  await act(async () => { await result.current.open("gpu", "/project", "/project/task.jsonl"); });
  expect(setActive).toHaveBeenCalledWith("retry");
});
