// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MockPort } from "../../port/mock";
import { HttpError } from "../../port/port";
import { useWorkbenchFiles } from "../useWorkbenchFiles";

afterEach(() => { cleanup(); vi.useRealTimers(); });

it.each([[404, "workspace.file_missing"], [403, "workspace.preview_denied"]])("surfaces a %s poll failure and removes stale saved text", async (status, code) => {
  vi.useFakeTimers();
  const port = new MockPort();
  const read = vi.spyOn(port, "workspaceFile").mockResolvedValue({ path: "a.md", content: "saved", revision: "r1" });
  const paths = ["a.md"];
  const { result } = renderHook(() => useWorkbenchFiles({ port, workspaceRoot: "/work", path: "a.md", openPaths: paths, shown: true, refresh: "", poll: true }));
  await act(async () => {});
  expect(result.current.draft).toBe("saved");
  read.mockRejectedValue(new HttpError(status, "unavailable", { code }));
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(result.current.errorCode).toBe(code);
  expect(result.current.file).toBeNull();
  expect(result.current.draft).toBe("");
  read.mockResolvedValue({ path: "a.md", content: "recreated", revision: "r2" });
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(result.current.draft).toBe("recreated");
  expect(result.current.error).toBe("");
});

it("does not poll metadata for every keystroke and preserves a draft after deletion", async () => {
  vi.useFakeTimers();
  const port = new MockPort();
  vi.spyOn(port, "workspaceFile").mockResolvedValue({ path: "a.md", content: "saved", revision: "r1", observedVersion: "v1" });
  const stat = vi.spyOn(port, "workspacePreviewInfo").mockResolvedValue({ ref: { path: "a.md" }, kind: "markdown", version: "v1", size: 5, limit: 2 << 20, readOnly: false });
  const paths = ["a.md"];
  const { result } = renderHook(() => useWorkbenchFiles({ port, workspaceRoot: "/work", path: "a.md", openPaths: paths, shown: true, refresh: "", poll: true }));
  await act(async () => {});
  for (const text of ["a", "ab", "abc"]) await act(async () => result.current.setDraft(text));
  expect(stat).toHaveBeenCalledOnce();
  stat.mockRejectedValue(new HttpError(404, "gone", { code: "workspace.file_missing" }));
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(result.current.draft).toBe("abc");
  expect(result.current.error).toContain("找不到");
});
