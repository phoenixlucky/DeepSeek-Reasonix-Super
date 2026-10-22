// @vitest-environment jsdom
import "./testkit";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { CompositionEditor } from "./CompositionEditor";
import { CompositionModulePicker } from "./CompositionModulePicker";
import { builtinCompositions, type AgentComposition, type AgentPresetCatalog, type CompositionModule } from "../port/composition";

afterEach(cleanup);
const modules = {
  tools: [{ id: "bash", description: "Run a shell command", source: "builtin" }, { id: "task", source: "builtin" }],
  skills: [{ id: "review", description: "Review changes", source: "builtin" }, { id: "notes", description: "Tidy notes", source: "custom" }, { id: "release", source: "custom" }],
  mcp: [{ id: "docs", source: "custom" }, { id: "repo", source: "custom" }],
} satisfies AgentPresetCatalog["modules"];

function open(initial?: (draft: AgentComposition) => void) {
  let current = builtinCompositions()[0];
  current.id = "user:picker";
  current.skills = { mode: "select", members: [] };
  initial?.(current);
  let valid = true;
  const props = () => ({ draft: current, modules, disabled: false, creating: true,
    onChange: (next: AgentComposition) => { current = next; view.rerender(<CompositionEditor {...props()} />); },
    onValidityChange: (next: boolean) => { valid = next; },
  });
  const view = render(<CompositionEditor {...props()} />);
  return { get saved() { return current; }, get valid() { return valid; } };
}

async function picker(label = "技能 添加模块") {
  await userEvent.click(screen.getByText("技能与扩展"));
  await userEvent.click(screen.getByRole("button", { name: label }));
  return screen.getByRole("dialog", { name: label });
}

it("keeps a grouped checkbox menu open for multiple selections and stores only ids", async () => {
  const state = open();
  const menu = await picker();
  expect(within(menu).getByRole("group", { name: "内置" })).toBeTruthy();
  expect(within(menu).getByRole("group", { name: "自定义" })).toBeTruthy();
  expect(within(menu).getByText("Review changes")).toBeTruthy();
  await userEvent.click(within(menu).getByRole("checkbox", { name: "review" }));
  await userEvent.click(within(menu).getByRole("checkbox", { name: "notes" }));
  expect(state.saved.skills.members).toEqual([{ id: "review" }, { id: "notes" }]);
  expect(within(menu).getByRole<HTMLInputElement>("checkbox", { name: "全选（3 项）" }).indeterminate).toBe(true);
  await userEvent.click(within(menu).getByRole("button", { name: "完成" }));
  expect(screen.queryByRole("dialog", { name: "技能 添加模块" })).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "技能 添加模块" }));
});

it("selects groups and filtered results without removing hidden selections", async () => {
  const state = open();
  const menu = await picker();
  const custom = within(menu).getByRole("group", { name: "自定义" });
  await userEvent.click(within(custom).getByRole("button", { name: "全选本组" }));
  expect(state.saved.skills.members.map(m => m.id)).toEqual(["notes", "release"]);
  await userEvent.click(within(menu).getByRole("checkbox", { name: "全选（3 项）" }));
  expect(state.saved.skills.members).toHaveLength(3);
  await userEvent.type(within(menu).getByRole("searchbox"), "Review");
  await userEvent.click(within(menu).getByRole("checkbox", { name: "全选搜索结果（1 项）" }));
  expect(state.saved.skills.members.map(m => m.id)).toEqual(["notes", "release"]);
  await userEvent.clear(within(menu).getByRole("searchbox"));
  await userEvent.type(within(menu).getByRole("searchbox"), "missing");
  expect(within(menu).getByText("没有匹配的模块")).toBeTruthy();
  expect(within(menu).getByRole<HTMLInputElement>("checkbox").disabled).toBe(true);
  await userEvent.click(within(menu).getByRole("button", { name: "清空" }));
  expect(state.saved.skills).toEqual({ mode: "select", members: [] });
});

it("preserves parameters and invalid JSON when earlier selections are removed", async () => {
  const state = open(d => { d.mcp = { mode: "select", members: [{ id: "docs", parameters: { args: ["--docs"] } }, { id: "repo", parameters: { args: ["--repo"] } }] }; });
  await userEvent.click(screen.getByText("技能与扩展"));
  const editor = screen.getByRole("textbox", { name: "MCP 服务 独立参数 2" });
  await userEvent.clear(editor);
  await userEvent.type(editor, "invalid");
  expect(state.valid).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "MCP 服务 添加模块" }));
  const menu = screen.getByRole("dialog", { name: "MCP 服务 添加模块" });
  await userEvent.click(within(menu).getByRole("checkbox", { name: "docs" }));
  await userEvent.click(within(menu).getByRole("button", { name: "完成" }));
  expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "MCP 服务 独立参数 1" }).value).toBe("invalid");
  expect(state.saved.mcp.members).toEqual([{ id: "repo", parameters: { args: ["--repo"] } }]);
  expect(state.valid).toBe(false);
});

it("supports Escape, keyboard selection, outside dismissal and unchanged inheritance", async () => {
  const state = open();
  let menu = await picker();
  await userEvent.keyboard("{ArrowDown} ");
  expect(state.saved.skills.members).toEqual([{ id: "review" }]);
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "技能 添加模块" })).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "技能 添加模块" }));
  menu = screen.getByRole("dialog", { name: "技能 添加模块" });
  await userEvent.click(screen.getByRole("textbox", { name: "预设名称" }));
  expect(menu.isConnected).toBe(false);
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "技能 选择方式" }), "inherit");
  expect(state.saved.skills).toEqual({ mode: "inherit", members: [] });
  expect(screen.queryByRole("button", { name: "技能 添加模块" })).toBeNull();
});

it("has one add-module picker and keeps manual IDs behind an explicit advanced entry", async () => {
  const state = open();
  await userEvent.click(screen.getByText("技能与扩展"));
  const group = screen.getByRole("group", { name: "技能" });
  expect(within(group).getAllByRole("button", { name: /添加模块/ })).toHaveLength(1);
  expect(within(group).getByRole("button", { name: "添加空白 ID" }).closest("details")?.open).toBe(false);
  await userEvent.click(within(group).getByText("高级：手动输入模块 ID"));
  await userEvent.click(within(group).getByRole("button", { name: "添加空白 ID" }));
  await userEvent.type(screen.getByRole("textbox", { name: "技能 ID 1" }), "private-skill");
  expect(state.saved.skills.members).toEqual([{ id: "private-skill" }]);
});

it("keeps legacy catalog choices unclassified, deduplicates ids and closes when disabled", async () => {
  let members: CompositionModule[] = [{ id: "saved-but-missing", parameters: { args: ["--keep"] } }];
  const onChange = (next: typeof members) => { members = next; };
  const props = { kind: "skills" as const, label: "技能", choices: [{ id: "old" }, { id: "old" }], members, disabled: false, onChange };
  const view = render(<CompositionModulePicker {...props} />);
  await userEvent.click(screen.getByRole("button", { name: "技能 添加模块" }));
  const menu = screen.getByRole("dialog", { name: "技能 添加模块" });
  expect(within(menu).getByRole("group", { name: "未分类" })).toBeTruthy();
  expect(within(menu).getAllByRole("checkbox", { name: "old" })).toHaveLength(1);
  await userEvent.click(within(menu).getByRole("checkbox", { name: "全选（1 项）" }));
  expect(members).toEqual([{ id: "saved-but-missing", parameters: { args: ["--keep"] } }, { id: "old" }]);
  view.rerender(<CompositionModulePicker {...props} disabled />);
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("shows empty catalogs without removing saved choices", async () => {
  render(<CompositionModulePicker kind="skills" label="技能" choices={[]} members={[{ id: "saved" }]} disabled={false} onChange={() => {}} />);
  await userEvent.click(screen.getByRole("button", { name: "技能 添加模块" }));
  expect(screen.getByText("暂无可选模块")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox").disabled).toBe(true);
  expect(screen.getByRole("button", { name: "清空" })).toBeTruthy();
});
