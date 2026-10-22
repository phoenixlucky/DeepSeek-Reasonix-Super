// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MockPort } from "../../port/mock";
import { HttpError } from "../../port/port";
import { useDocument } from "./useDocument";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("discards a previous file's late response and aborts on close", async () => {
  const port = new MockPort();
  let done!: (value: { data: ArrayBuffer; version: string }) => void;
  let signal: AbortSignal | undefined;
  vi.spyOn(port, "workspacePreviewContent").mockImplementationOnce((_ref, s?: AbortSignal) => {
    signal = s;
    return new Promise((resolve) => { done = resolve; });
  });
  const { result, rerender, unmount } = renderHook(({ path }) => useDocument(port, path, true, ""), { initialProps: { path: "one.csv" } });
  await waitFor(() => expect(done).toBeDefined());
  rerender({ path: "two.csv" });
  await waitFor(() => expect(result.current.value?.info.ref.path).toBe("two.csv"));
  expect(signal?.aborted).toBe(true);
  await act(async () => done({ data: new ArrayBuffer(0), version: "old" }));
  expect(result.current.value?.info.ref.path).toBe("two.csv");
  unmount();
});

it("removes stale bytes when access is revoked and reload fails", async () => {
  const port = new MockPort();
  const { result } = renderHook(() => useDocument(port, "report.csv", true, ""));
  await waitFor(() => expect(result.current.value?.data).toBeTruthy());
  vi.spyOn(port, "workspacePreviewInfo").mockRejectedValue(new HttpError(403, "denied", { code: "workspace.preview_denied" }));
  act(() => result.current.reload());
  await waitFor(() => expect(result.current.value?.error).toBeTruthy());
  expect(result.current.value?.data).toBeUndefined();
});

it("checks metadata on invalidation and only forces unchanged bytes for manual refresh", async () => {
  const port = new MockPort();
  const content = vi.spyOn(port, "workspacePreviewContent");
  const info = vi.spyOn(port, "workspacePreviewInfo");
  const { result, rerender } = renderHook(({ refresh }) => useDocument(port, "report.csv", true, refresh), { initialProps: { refresh: "first" } });
  await waitFor(() => expect(result.current.value?.data).toBeTruthy());
  rerender({ refresh: "tool finished" });
  await waitFor(() => expect(info).toHaveBeenCalledTimes(2));
  expect(content).toHaveBeenCalledTimes(1);
  act(() => result.current.reload());
  await waitFor(() => expect(content).toHaveBeenCalledTimes(2));
});
