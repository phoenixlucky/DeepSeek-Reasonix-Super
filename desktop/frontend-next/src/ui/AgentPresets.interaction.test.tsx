// @vitest-environment jsdom
import "./testkit";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, it, expect, vi } from "vitest";
import { MockPort } from "../port/mock";
import { AgentPresets } from "./AgentPresets";
import { boot, STORAGE, t } from "../i18n";
import { AgentCompositionPicker } from "./AgentCompositionPicker";
import { RunLine } from "./RunLine";
import { NEW_TASK_PRESET_KEY } from "../state/newTaskPreset";

afterEach(() => { cleanup(); vi.restoreAllMocks(); localStorage.removeItem("rx-show-agent-composition"); localStorage.removeItem(NEW_TASK_PRESET_KEY); localStorage.setItem(STORAGE, "zh"); boot(); });

it("persists the visibility toggle without selecting or changing the current composition", async () => {
  const port = new MockPort();
  const select = vi.spyOn(port, "selectAgentComposition");
  const user = userEvent.setup();
  render(<><AgentPresets port={port}/>
    <RunLine label="空闲" running={false} blocked={false} sent={0} received={0} estimated={false}>
      <AgentCompositionPicker port={port} running={false} onChanged={()=>{}} onSettings={()=>{}} onError={()=>{}}/>
    </RunLine>
  </>);
  await screen.findByRole("button", { name: "查看 日常搭档" });
  expect(screen.queryByRole("button", { name: /^Agent 组合/ })).toBeNull();
  const toggle = screen.getByRole("switch", { name: "显示模式选择器" });
  expect(toggle.getAttribute("aria-checked")).toBe("false");
  await user.click(toggle);
  const picker = await screen.findByRole("button", { name: /^Agent 组合/ });
  expect(picker.textContent).toBe("日常搭档");
  expect(localStorage.getItem("rx-show-agent-composition")).toBe("on");
  expect(await port.agentComposition()).toBeNull();
  expect(select).not.toHaveBeenCalled();
  await user.click(picker);
  await user.click(screen.getByRole("menuitem", { name: /轻装搭档/ }));
  expect((await port.agentComposition())?.id).toBe("builtin:minimal");
  expect(select).toHaveBeenCalledExactlyOnceWith("builtin:minimal");
  await user.click(toggle);
  expect(localStorage.getItem("rx-show-agent-composition")).toBe("off");
  expect(screen.queryByRole("button", { name: /^Agent 组合/ })).toBeNull();
  expect((await port.agentComposition())?.id).toBe("builtin:minimal");
  await user.click(toggle);
  expect((await screen.findByRole("button", { name: /^Agent 组合/ })).textContent).toBe("轻装搭档");
  expect(select).toHaveBeenCalledTimes(1);
});

it("restores an enabled choice and follows storage changes from another window", async () => {
  localStorage.setItem("rx-show-agent-composition", "on");
  const port = new MockPort();
  const select = vi.spyOn(port, "selectAgentComposition");
  render(<><AgentPresets port={port} /><RunLine label="空闲" running={false} blocked={false} sent={0} received={0} estimated={false}>
    <AgentCompositionPicker port={port} running={false} onChanged={()=>{}} onSettings={()=>{}} onError={()=>{}} />
  </RunLine></>);
  await screen.findByRole("button", { name: /^Agent 组合/ });
  expect(screen.getByRole("switch", { name: "显示模式选择器" }).getAttribute("aria-checked")).toBe("true");
  act(() => {
    localStorage.removeItem("rx-show-agent-composition");
    window.dispatchEvent(new StorageEvent("storage", { key: "rx-show-agent-composition" }));
  });
  expect(screen.getByRole("switch", { name: "显示模式选择器" }).getAttribute("aria-checked")).toBe("false");
  expect(screen.queryByRole("button", { name: /^Agent 组合/ })).toBeNull();
  expect(select).not.toHaveBeenCalled();
});

it("keeps visibility off and reports a refused persistence write", async () => {
  render(<AgentPresets port={new MockPort()} />);
  await screen.findByRole("button", { name: "查看 日常搭档" });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("storage denied"); });
  await userEvent.click(screen.getByRole("switch", { name: "显示模式选择器" }));
  expect(screen.getByRole("switch", { name: "显示模式选择器" }).getAttribute("aria-checked")).toBe("false");
  expect(screen.getByRole("alert").textContent).toContain("无法保存模式选择器设置");
});

it("copies both built-in name and description in the window language", async () => {
  localStorage.setItem(STORAGE, "en"); boot();
  const port = new MockPort();
  render(<AgentPresets port={port} />);
  await userEvent.click(await screen.findByRole("button", { name: "Duplicate Everyday partner" }));
  await userEvent.click(screen.getByRole("button", { name: "Save preset" }));
  await screen.findByRole("status");
  const catalog = await port.agentPresets();
  const base = catalog.presets.find((v) => v.definition.id === "builtin:general")!.definition;
  const copy = catalog.presets.find((v) => v.definition.id === "user:custom")!.definition;
  expect(copy.name).toBe("Everyday partner Copy");
  expect(copy.description).toBe(t(base.description));
});

describe("Agent preset management", () => {
  it("edits the existing user persona in Standard without migration or binding", async () => {
    const user = userEvent.setup();
    const port = new MockPort();
    render(<AgentPresets port={port} />);
    await user.click(await screen.findByRole("button", { name: "查看 日常搭档" }));
    await user.click(screen.getByText("更多操作"));
    await user.click(screen.getByRole("button", { name: "编辑日常搭档人设" }));
    await user.type(await screen.findByLabelText("系统提示词开头"), "User identity");
    await user.click(screen.getByRole("button", { name: "保存" }));
    await screen.findByRole("button", { name: "新建预设" });
    expect((await port.persona()).prefix).toBe("User identity");
    expect(await port.agentComposition()).toBeNull();
    expect((await port.agentPresets()).presets).toHaveLength(2);
  });
  it("opens the user persona from settings search even with a different preferred preset", async () => {
    const port = new MockPort();
    await port.setDefaultAgentPreset("builtin:minimal", false);
    render(<AgentPresets port={port} focusPersona />);
    expect(await screen.findByLabelText("系统提示词开头")).toBeTruthy();
    expect(screen.getByRole("button", { name: "严谨工程师" })).toBeTruthy();
    expect(await port.agentComposition()).toBeNull();
  });
  it("shows only Standard and Minimal built-in cards and changes the default without changing this conversation", async () => {
    const user = userEvent.setup();
    const port = new MockPort();
    render(<AgentPresets port={port} />);
    expect(await screen.findByRole("button", { name: "查看 日常搭档" })).toBeTruthy();
    const minimal = screen.getByRole("button", { name: "查看 轻装搭档" });
    expect(within(screen.getByRole("region", { name: "内置" })).getAllByRole("article")).toHaveLength(2);
    expect(screen.queryByText("代码审查")).toBeNull();
    expect(screen.queryByText("资料研究")).toBeNull();
    await user.click(minimal);
    await user.click(screen.getByText("更多操作"));
    await user.click(screen.getByText("显式默认配置"));
    await user.click(screen.getByRole("button", { name: "设为个人首选" }));
    await waitFor(async () => expect((await port.agentPresets()).default).toBe("builtin:minimal"));
    expect(await port.agentComposition()).toBeNull();
    await user.click(screen.getByRole("button", { name: "复制为自定义预设" }));
    await user.click(screen.getByText("技能与扩展", { selector: "summary" }));
    expect(screen.getByText("安全 Hook 和权限规则始终生效；此处仅选择非阻断型自动化。")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "保存预设" }));
    await screen.findByRole("status");
    const copied = (await port.agentPresets()).presets.find((v) => v.definition.id === "user:custom")!;
    expect(copied.definition.tools.members.map((m) => m.id)).toEqual(["bash", "view_image"]);
    expect(copied.definition.mcp).toEqual({ mode: "select", members: [] });
  });
  it("copies a built-in, saves literal fields and keeps the current session revision", async () => {
    const user = userEvent.setup();
    const port = new MockPort();
    render(<AgentPresets port={port} />);
    await user.click(await screen.findByRole("button", { name: "复制 日常搭档" }));
    await user.clear(screen.getByLabelText("预设名称"));
    await user.type(screen.getByLabelText("预设名称"), "My agent");
    await user.type(screen.getByLabelText("人设与要求"), "Literal prefix");
    await user.type(screen.getByLabelText("补充要求"), "Literal suffix");
    expect(screen.queryByRole("switch", { name: /complete/ })).toBeNull();
    await user.click(screen.getByRole("switch", { name: "不注入可选运行时上下文" }));
    await user.click(screen.getByRole("button", { name: "保存预设" }));
    expect((await screen.findByRole("status")).textContent).toContain("新会话使用新版");
    expect((await port.agentPresets()).presets.some((v) => v.definition.name === "My agent")).toBe(true);
    expect(await port.agentComposition()).toBeNull();
    expect((await port.agentPresets()).presets.find((v)=>v.definition.name==="My agent")?.definition.persona).toEqual({prefix:"Literal prefix",suffix:"Literal suffix",include_runtime_context:false});
  });
  it("keeps an unsaved draft after a conflict", async () => {
    const user = userEvent.setup();
    const port = new MockPort();
    vi.spyOn(port, "saveAgentPreset").mockRejectedValue(new Error("conflict"));
    render(<AgentPresets port={port} />);
    await user.click(await screen.findByRole("button", { name: "复制 日常搭档" }));
    await user.type(screen.getByLabelText("预设名称"), " draft");
    await user.click(screen.getByRole("button", { name: "保存预设" }));
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect((screen.getByLabelText("预设名称") as HTMLInputElement).value).toContain("draft");
  });
  it("does not save an earlier parameter value when the visible JSON is invalid", async () => {
    const user=userEvent.setup();const port=new MockPort();const save=vi.spyOn(port,"saveAgentPreset");
    render(<AgentPresets port={port}/>);
    await user.click(await screen.findByRole("button",{name:"复制 日常搭档"}));
    await user.click(screen.getByText("技能与扩展", { selector: "summary" }));
    await user.selectOptions(screen.getByLabelText("MCP 服务 选择方式"),"select");
    const group=screen.getByRole("group",{name:"MCP 服务"});
    await user.click(within(group).getByText("高级：手动输入模块 ID"));
    await user.click(within(group).getByRole("button", { name: "添加空白 ID" }));
    await user.type(screen.getByLabelText("MCP 服务 ID 1"),"docs");
    await user.clear(screen.getByLabelText("MCP 服务 独立参数 1"));
    await user.type(screen.getByLabelText("MCP 服务 独立参数 1"),"invalid");
    await user.selectOptions(screen.getByLabelText("存放范围"), "project");
    expect((screen.getByLabelText("MCP 服务 独立参数 1") as HTMLTextAreaElement).value).toBe("invalid");
    expect(screen.getByRole<HTMLButtonElement>("button",{name:"保存预设"}).disabled).toBe(true);
    expect(save).not.toHaveBeenCalled();
    await user.click(group.querySelector(".composition-module button")!);
    await waitFor(()=>expect(screen.getByRole<HTMLButtonElement>("button",{name:"保存预设"}).disabled).toBe(false));
  });
});
