// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Workspaces } from "./Workspaces";
import { RailSearch } from "./railsearch";
import { HttpError } from "../port/port";
import type { TreeSession, TreeWorkspace } from "../port/hub";

afterEach(cleanup);

const A: TreeSession = { path: "/a/s/one.jsonl", name: "one", title: "alpha plan" };
const tree = (extra: TreeWorkspace[] = [{ root: "/b", name: "beta", remembered: true, sessions: [] }]): TreeWorkspace[] => [
  { root: "/a", name: "alpha-project", remembered: true, sessions: [A] }, ...extra,
];

function draw(workspaces = tree(), session = A, moveSessionTo = vi.fn(async () => ({ path: "/b/s/one.jsonl", originalRemoved: true }))) {
  const reload = vi.fn().mockResolvedValue(undefined);
  const onError = vi.fn();
  workspaces[0].sessions = [session];
  render(<RailSearch><Workspaces
    hub={{ moveSessionTo } as never} tree={workspaces} treeRead runtimes={[]} active="" folded={new Set()} onFold={() => {}} reload={reload}
    onOpen={vi.fn().mockResolvedValue(undefined)} onFocus={() => {}} onClose={vi.fn().mockResolvedValue(undefined)}
    liveIds={() => []} runs={{}} scope="all" onArchive={vi.fn()} onRename={() => {}} onError={onError}
    adder={{ add: () => {}, close: () => {}, at: null } as never}
  /></RailSearch>);
  return { moveSessionTo, reload, onError };
}

const openDialog = () => {
  fireEvent.contextMenu(screen.getByRole("treeitem", { name: /alpha plan/ }));
  fireEvent.click(screen.getByRole("menuitem", { name: "移动到项目…" }));
  return screen.getByRole("dialog");
};

it("offers the move in a closed conversation's menu and not in an open one's", () => {
  draw();
  fireEvent.contextMenu(screen.getByRole("treeitem", { name: /alpha plan/ }));
  expect((screen.getByRole("menuitem", { name: "移动到项目…" }) as HTMLButtonElement).disabled).toBe(false);
  cleanup();
  draw(tree(), { ...A, runtimeId: "r1" });
  fireEvent.contextMenu(screen.getByRole("treeitem", { name: /alpha plan/ }));
  expect((screen.getByRole("menuitem", { name: "移动到项目…" }) as HTMLButtonElement).disabled).toBe(true);
});

it("asks where, lists the other projects only, and says what travels and what does not", () => {
  draw(tree([{ root: "/b", name: "beta", remembered: true, sessions: [] }, { root: "/c", name: "gamma", remembered: true, sessions: [] }, { root: "/p", name: "pane-only", sessions: [] }]));
  const dialog = openDialog();
  const options = [...dialog.querySelectorAll("option")].map((o) => o.textContent);
  expect(options).toEqual(["beta — /b", "gamma — /c"]);
  expect(dialog.textContent).toContain("原项目的权限与目标设置不会跟着走");
  expect(dialog.textContent).toContain("对话里记录的路径保持原样");
  expect(dialog.textContent).toContain("检查点（回退快照）、后台任务日志和排队中的消息不随会话移动，原件会被移除");
});

it("moves through the kernel once, reads the tree again and closes", async () => {
  const { moveSessionTo, reload } = draw();
  openDialog();
  await userEvent.click(screen.getByRole("button", { name: "移动" }));
  expect(moveSessionTo).toHaveBeenCalledExactlyOnceWith(A.path, "/b");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(reload).toHaveBeenCalledTimes(1);
});

it("keeps move-to beside pin, archive and ordering in the keyboard menu", async () => {
  const { moveSessionTo } = draw();
  screen.getByRole("treeitem", { name: /alpha plan/ }).focus();
  await userEvent.keyboard("{Shift>}{F10}{/Shift}");
  for (const name of ["置顶会话", "归档会话", "上移", "下移"]) {
    expect(screen.getByRole("menuitem", { name })).toBeTruthy();
  }
  await userEvent.click(screen.getByRole("menuitem", { name: "移动到项目…" }));
  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement?.getAttribute("aria-label")).toBe("目标项目");
  await userEvent.click(screen.getByRole("button", { name: "移动" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(moveSessionTo).toHaveBeenCalledExactlyOnceWith(A.path, "/b");
});

it("keeps the dialog open and says why when the kernel refuses", async () => {
  const refuse = vi.fn(async () => { throw new HttpError(409, "held", { code: "session.move_held", error: "held" }); });
  const { reload } = draw(tree(), A, refuse as never);
  openDialog();
  await userEvent.click(screen.getByRole("button", { name: "移动" }));
  expect((await screen.findByRole("alert")).textContent).toContain("该会话还在被写入，稍后再移动");
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(reload).not.toHaveBeenCalled();
});

it("says so when the copy arrived but the original could not be removed", async () => {
  const partial = vi.fn(async () => ({ path: "/b/s/one.jsonl", originalRemoved: false }));
  draw(tree(), A, partial as never);
  openDialog();
  await userEvent.click(screen.getByRole("button", { name: "移动" }));
  expect((await screen.findByRole("status")).textContent).toContain("原来的那份没能删除");
  expect(screen.queryByRole("button", { name: "移动" })).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("has nowhere to move to when there is no other project", () => {
  draw(tree([]));
  const dialog = openDialog();
  expect(dialog.textContent).toContain("没有别的项目可以移到");
  expect((screen.getByRole("button", { name: "移动" }) as HTMLButtonElement).disabled).toBe(true);
});

it("takes focus when it opens, on the project choice, or on Cancel when there is nothing to choose", () => {
  draw();
  const dialog = openDialog();
  expect(dialog.contains(document.activeElement)).toBe(true);
  expect(document.activeElement?.getAttribute("aria-label")).toBe("目标项目");
  cleanup();
  draw(tree([]));
  const empty = openDialog();
  expect(empty.contains(document.activeElement)).toBe(true);
  expect(document.activeElement?.textContent).toBe("取消");
});
