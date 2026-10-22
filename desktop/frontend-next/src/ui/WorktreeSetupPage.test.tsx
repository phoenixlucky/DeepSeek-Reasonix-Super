// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorktreeSetupPage } from "./WorktreeSetupPage";
import type { WorktreePreparation } from "../port/workspace";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const status = (phase: string): WorktreePreparation => ({ id: "operation", phase, ref: "refs/heads/main", sourceRoot: "/source/project", workspaceRoot: "/worktrees/project", branch: "reasonix/task", startedAt: Date.now() });
const draw = (phase: string, code?: string) => {
  const retry = vi.fn(), cancel = vi.fn(), edit = vi.fn();
  const view = render(<WorktreeSetupPage stage={{ ...status(phase), code, detail: "diagnostic" }} task="保留的任务" files={["input.txt"]} busy={false} onRetry={retry} onCancel={cancel} onEdit={edit}/>);
  return { ...view, retry, cancel, edit };
};

it("retains task and attachments during a long wait and lets the user cancel", () => {
  vi.useFakeTimers();
  const { cancel } = draw("checking_out");
  expect(screen.getByText("保留的任务")).toBeTruthy();
  expect(screen.getByText("input.txt")).toBeTruthy();
  act(() => vi.advanceTimersByTime(16000));
  expect(screen.getByRole("status").textContent).toContain("仍在等待");
  fireEvent.click(screen.getByRole("button", { name: "取消创建" }));
  expect(cancel).toHaveBeenCalledOnce();
});

it("keeps failure details behind disclosure and retries without losing the task", () => {
  const { retry } = draw("failed", "workspace.worktree_timeout");
  expect(screen.queryByText("diagnostic")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "查看详情" }));
  expect(screen.getByText("diagnostic")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试创建" }));
  expect(retry).toHaveBeenCalledOnce();
  expect(screen.getByText("保留的任务")).toBeTruthy();
});

it("does not offer another creation while cleanup is uncertain", () => {
  draw("cleanup_failed", "workspace.worktree_cleanup_failed");
  expect(screen.queryByRole("button", { name: "重试创建" })).toBeNull();
  const cancel = screen.queryByRole("button", { name: "取消创建" }) as HTMLButtonElement | null;
  expect(cancel === null || cancel.disabled).toBe(true);
  expect(screen.getByRole("note").textContent).toContain("会话菜单");
  expect(screen.getByRole("note").textContent).toContain("源目录");
  expect(screen.getByText("已保留")).toBeTruthy();
});

it("restores editing after confirmed cancellation and exposes the retained destination", () => {
  const { edit } = draw("cancelled");
  fireEvent.click(screen.getByRole("button", { name: "返回编辑" }));
  expect(edit).toHaveBeenCalledOnce();
  expect(screen.getByText("input.txt")).toBeTruthy();
});

it("does not claim automatic submission when a late cancellation retained the workspace", () => {
  draw("retained");
  expect(screen.getByText("目录已经创建成功，取消未能撤回目录切换。任务尚未发送。")).toBeTruthy();
  expect(screen.queryByText("即将进入对话，自动开始任务。")).toBeNull();
});

it("moves keyboard focus to the preparation heading on entry", () => {
  draw("checking_out");
  expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 }));
});

it("recovers focus when cancellation removes its button without stealing focus from details", () => {
  const { rerender } = draw("checking_out");
  const props = { task: "task", files: [], busy: false, onRetry: vi.fn(), onCancel: vi.fn(), onEdit: vi.fn() };
  screen.getByRole("button", { name: "查看详情" }).focus();
  rerender(<WorktreeSetupPage {...props} stage={status("creating")}/>);
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "查看详情" }));
  screen.getByRole("button", { name: "取消创建" }).focus();
  rerender(<WorktreeSetupPage {...props} stage={status("cancelled")}/>);
  expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 }));
});

it.each(["failed", "cancelled", "retained", "ready", "unreachable", "cleanup_failed"])("does not describe %s as still waiting", (phase) => {
  draw(phase);
  expect(screen.queryByText(/已等待 \d+ 秒/)).toBeNull();
});

it("names retained cleanup failure directly even without an error code", () => {
  draw("cleanup_failed");
  expect(screen.getByText("清理未完成")).toBeTruthy();
  expect(screen.queryByText("正在等待状态确认")).toBeNull();
});

it("identifies details disclosure separately from workspace configuration", () => {
  draw("failed");
  expect(screen.getByRole("button", { name: "查看详情" }).getAttribute("data-action")).toBe("workspace.prepare-details");
  expect(screen.getByRole("button", { name: "返回编辑" }).getAttribute("data-action")).toBe("workspace.prepare-edit");
});
