// @vitest-environment jsdom
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useAddWorkspace } from "./addws";
import type { HubPort } from "../port/hub";

type Capabilities = { pickFolder: boolean; addWorkspace: boolean };

function hub(
  pickFolder: () => Promise<string | null>,
  capabilities: () => Promise<Capabilities> = async () => ({ pickFolder: true, addWorkspace: true }),
  addWorkspace: (path: string) => Promise<unknown> = vi.fn(async () => ({})),
) {
  return { pickFolder, hostCapabilities: capabilities, addWorkspace } as unknown as HubPort;
}

describe("adding a workspace", () => {
  it("adds the folder returned by the native picker", async () => {
    const port = hub(async () => "D:\\work\\project");
    const reload = vi.fn(async () => undefined);
    const fail = vi.fn();
    const { result } = renderHook(() => useAddWorkspace(port, reload, fail));

    act(() => result.current.add());

    await waitFor(() => expect(port.addWorkspace).toHaveBeenCalledWith("D:\\work\\project"));
    expect(reload).toHaveBeenCalledOnce();
    expect(fail).not.toHaveBeenCalled();
  });

  it("falls back to a path field when the server has no native picker", async () => {
    const port = hub(async () => null, async () => ({ pickFolder: false, addWorkspace: true }));
    const reload = vi.fn(async () => undefined);
    const fail = vi.fn();
    const { result } = renderHook(() => useAddWorkspace(port, reload, fail));

    act(() => result.current.add());

    await waitFor(() => expect(result.current.pathOpen).toBe(true));
    act(() => result.current.addPath("/srv/project"));

    await waitFor(() => expect(port.addWorkspace).toHaveBeenCalledWith("/srv/project"));
    expect(reload).toHaveBeenCalledOnce();
    expect(fail).not.toHaveBeenCalled();
  });

  it("reports a path the kernel refuses without claiming the window is the problem", async () => {
    const refusal = new Error("no such directory");
    const port = hub(
      async () => null,
      async () => ({ pickFolder: false, addWorkspace: true }),
      vi.fn(async () => { throw refusal; }),
    );
    const fail = vi.fn();
    const { result } = renderHook(() => useAddWorkspace(port, vi.fn(), fail));

    act(() => result.current.add());
    await waitFor(() => expect(result.current.pathOpen).toBe(true));
    act(() => result.current.addPath("/srv/missing"));

    await waitFor(() => expect(fail).toHaveBeenCalledWith(refusal));
  });

  it("says when the kernel cannot add workspaces at all", async () => {
    const port = hub(async () => null, async () => ({ pickFolder: false, addWorkspace: false }));
    const fail = vi.fn();
    const { result } = renderHook(() => useAddWorkspace(port, vi.fn(), fail));

    act(() => result.current.add());

    await waitFor(() => expect(fail).toHaveBeenCalledOnce());
    expect(String(fail.mock.calls[0]?.[0])).toContain("不支持添加工作区");
  });

  describe("the folder that was just added", () => {
    const added = { root: "/srv/project", name: "project", remembered: true, sessions: [] };

    it("is handed to the owner once the tree has been re-read, whichever way it was chosen", async () => {
      const order: string[] = [];
      const picked = hub(async () => "/srv/project", undefined, vi.fn(async () => added));
      const onAdded = vi.fn(async () => { order.push("opened"); });
      const reload = vi.fn(async () => { order.push("reloaded"); });
      const { result } = renderHook(() => useAddWorkspace(picked, reload, vi.fn(), onAdded));

      act(() => result.current.add());
      await waitFor(() => expect(onAdded).toHaveBeenCalledWith(added));
      expect(order).toEqual(["reloaded", "opened"]);

      const typed = hub(async () => null, async () => ({ pickFolder: false, addWorkspace: true }), vi.fn(async () => added));
      const again = vi.fn(async () => undefined);
      const second = renderHook(() => useAddWorkspace(typed, vi.fn(), vi.fn(), again));
      act(() => second.result.current.add());
      await waitFor(() => expect(second.result.current.pathOpen).toBe(true));
      act(() => second.result.current.addPath("/srv/project"));
      await waitFor(() => expect(again).toHaveBeenCalledWith(added));
    });

    it("is not opened when the add was refused, and a refused open is reported", async () => {
      const refused = hub(async () => "/srv/x", undefined, vi.fn(async () => { throw new Error("no"); }));
      const onAdded = vi.fn();
      const fail = vi.fn();
      const first = renderHook(() => useAddWorkspace(refused, vi.fn(), fail, onAdded));
      act(() => first.result.current.add());
      await waitFor(() => expect(fail).toHaveBeenCalledOnce());
      expect(onAdded).not.toHaveBeenCalled();

      const port = hub(async () => "/srv/project", undefined, vi.fn(async () => added));
      const broken = new Error("cannot open");
      const failOpen = vi.fn();
      const second = renderHook(() => useAddWorkspace(port, vi.fn(async () => undefined), failOpen, async () => { throw broken; }));
      act(() => second.result.current.add());
      await waitFor(() => expect(failOpen).toHaveBeenCalledWith(broken));
    });
  });
});
