// @vitest-environment jsdom
import "./testkit";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { CompositionEditor } from "./CompositionEditor";
import { MockPort } from "../port/mock";
import type { AgentComposition, AgentPresetCatalog } from "../port/composition";

afterEach(cleanup);

async function open() {
  const port = new MockPort();
  const catalog: AgentPresetCatalog = await port.agentPresets();
  const draft: AgentComposition = structuredClone(catalog.presets.find((v) => v.definition.id.startsWith("user:") || true)!.definition);
  draft.id = "user:picker";
  draft.tools = { mode: "select", members: [] };
  let current = draft;
  const view = render(
    <CompositionEditor draft={current} modules={catalog.modules} disabled={false} creating
      onChange={(next: AgentComposition) => { current = next; view.rerender(<CompositionEditor draft={current} modules={catalog.modules} disabled={false} creating onChange={() => {}} onValidityChange={() => {}} />); }}
      onValidityChange={() => {}} />,
  );
  return { catalog, get saved() { return current; } };
}

it("shows what a module is for beside its name, and says nothing where its source says nothing", async () => {
  await open();
  await userEvent.click(screen.getByText("工具", { selector: "summary" }));
  await userEvent.click(screen.getByRole("button", { name: "工具 添加模块" }));
  expect(screen.getByText("Run a shell command in the workspace and return its output.")).toBeTruthy();
  expect(screen.getByRole("checkbox", { name: "task" }).getAttribute("aria-describedby")).toBeNull();
});

it("stores only the id of the module it adds, never its description", async () => {
  const state = await open();
  await userEvent.click(screen.getByText("工具", { selector: "summary" }));
  await userEvent.click(screen.getByRole("button", { name: "工具 添加模块" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "bash" }));
  expect(state.saved.tools.members).toEqual([{ id: "bash" }]);
});
