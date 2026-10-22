// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MockPort } from "../../port/mock";
import { useWorkbenchFiles } from "../useWorkbenchFiles";

afterEach(cleanup);

it("warns about disk changes using metadata without reloading an unsaved draft", async () => {
  const port = new MockPort();
  const read = vi.spyOn(port, "workspaceFile").mockResolvedValue({ path: "notes.md", content: "# Saved", revision: "hash", observedVersion: "disk-1" });
  const stat = vi.spyOn(port, "workspacePreviewInfo").mockResolvedValue({
    ref: { path: "notes.md" }, kind: "markdown", version: "disk-2", size: 9, limit: 2 << 20, readOnly: false,
  });
  const paths = ["notes.md"];
  const { result } = renderHook(() => useWorkbenchFiles({
    port, workspaceRoot: "/fixture", path: "notes.md", openPaths: paths, shown: true, refresh: "", poll: false,
  }));
  await waitFor(() => expect(result.current.file).toBeTruthy());
  act(() => result.current.setDraft("# My unsaved changes"));
  await waitFor(() => expect(result.current.error).toContain("未保存的编辑已保留"));
  expect(result.current.draft).toBe("# My unsaved changes");
  expect(stat).toHaveBeenCalled();
  expect(read).toHaveBeenCalledOnce();
});
