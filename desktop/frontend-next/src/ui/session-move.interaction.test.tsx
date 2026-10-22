// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Workspaces } from "./Workspaces";
import { RailSearch } from "./railsearch";
import { useSessionMove } from "./useSessionMove";
import type { TreeSession, TreeWorkspace } from "../port/hub";

afterEach(cleanup);

const rows = (sessions: TreeSession[]): TreeSession[] => sessions;
const A: TreeSession = { path: "/w/s/a.jsonl", name: "a", title: "alpha plan" };
const B: TreeSession = { path: "/w/s/b.jsonl", name: "b", title: "beta review" };
const C: TreeSession = { path: "/w/s/c.jsonl", name: "c", title: "gamma notes" };

function draw(sessions: TreeSession[] = [A, B, C], fail?: Error) {
  const tree: TreeWorkspace[] = [{ root: "/w", name: "w", sessions: rows(sessions) }];
  const moveSession = vi.fn(async () => { if (fail) throw fail; });
  const reload = vi.fn().mockResolvedValue(undefined);
  const onError = vi.fn();
  render(<RailSearch><Workspaces
    hub={{ moveSession } as never} tree={tree} treeRead runtimes={[]} active="" folded={new Set()} onFold={() => {}} reload={reload}
    onOpen={vi.fn().mockResolvedValue(undefined)} onFocus={() => {}} onClose={vi.fn().mockResolvedValue(undefined)}
    liveIds={() => []} runs={{}} scope="all" onArchive={vi.fn()} onRename={() => {}} onError={onError}
    adder={{ add: () => {}, close: () => {}, at: null } as never}
  /></RailSearch>);
  return { moveSession, reload, onError };
}

const row = (title: string) => screen.getByRole("treeitem", { name: new RegExp(title) });
const item = (title: string, name: string) => {
  fireEvent.contextMenu(row(title));
  return screen.getByRole("menuitem", { name }) as HTMLButtonElement;
};

it("offers one step up and down in a session's menu, and not past either end", () => {
  draw();
  expect(item("alpha plan", "上移").disabled).toBe(true);
  expect(item("alpha plan", "下移").disabled).toBe(false);
  cleanup();
  draw();
  expect(item("gamma notes", "下移").disabled).toBe(true);
  expect(item("gamma notes", "上移").disabled).toBe(false);
});

it("moves through the kernel once, then asks the tree again rather than patching it", async () => {
  const { moveSession, reload } = draw();
  await userEvent.click(item("beta review", "上移"));
  expect(moveSession).toHaveBeenCalledExactlyOnceWith(B.path, -1);
  expect(reload).toHaveBeenCalledTimes(1);
});

it("keeps keyboard menus and move shortcuts available on the same row", async () => {
  const { moveSession, reload } = draw();
  row("beta review").focus();
  await userEvent.keyboard("{Shift>}{F10}{/Shift}");
  expect(screen.getByRole("menuitem", { name: "置顶会话" })).toBeTruthy();
  expect(screen.getByRole("menuitem", { name: "归档会话" })).toBeTruthy();
  await userEvent.click(screen.getByRole("menuitem", { name: "上移" }));
  expect(moveSession).toHaveBeenCalledExactlyOnceWith(B.path, -1);
  expect(reload).toHaveBeenCalledTimes(1);
  row("beta review").focus();
  await userEvent.keyboard("{Alt>}{ArrowDown}{/Alt}");
  expect(document.activeElement).toBe(row("beta review"));
  expect(moveSession).toHaveBeenLastCalledWith(B.path, 1);
  expect(moveSession).toHaveBeenCalledTimes(2);
});

it("moves with Alt+Arrow on the focused row, and ignores a step that has nowhere to go", async () => {
  const { moveSession } = draw();
  fireEvent.keyDown(row("beta review"), { key: "ArrowDown", altKey: true });
  await Promise.resolve();
  expect(moveSession).toHaveBeenCalledExactlyOnceWith(B.path, 1);
  fireEvent.keyDown(row("alpha plan"), { key: "ArrowUp", altKey: true });
  fireEvent.keyDown(row("gamma notes"), { key: "ArrowDown", altKey: true });
  fireEvent.keyDown(row("beta review"), { key: "ArrowDown" });
  expect(moveSession).toHaveBeenCalledTimes(1);
});

it("treats archived and active rows as separate lists, so a step never passes a row the person cannot see", () => {
  const archived: TreeSession = { path: "/w/s/z.jsonl", name: "z", title: "zeta archived", archived: true };
  const ws: TreeWorkspace = { root: "/w", name: "w", sessions: [A, archived, B] };
  const { result } = renderHook(() => useSessionMove({ moveSession: vi.fn() } as never, vi.fn(), vi.fn(), false));
  expect(result.current.canMove(ws, B)).toEqual({ up: true, down: false });
  expect(result.current.canMove(ws, A)).toEqual({ up: false, down: true });
  expect(result.current.canMove(ws, archived)).toEqual({ up: false, down: false });
});

it("says so when the kernel refuses a move and leaves the list as it was", async () => {
  const { onError, reload } = draw([A, B, C], new Error("no room"));
  await userEvent.click(item("beta review", "下移"));
  expect(onError).toHaveBeenCalledTimes(1);
  expect(reload).not.toHaveBeenCalled();
});

it("has no order to move within while a search narrows the list", async () => {
  draw();
  await userEvent.type(screen.getByRole("searchbox"), "a");
  expect(item("alpha plan", "下移").disabled).toBe(true);
});
