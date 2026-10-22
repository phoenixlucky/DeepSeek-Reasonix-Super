// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AgentPort } from "../port/port";
import { HttpError } from "../port/http_error";
import type { HubPort, RuntimeView } from "../port/hub";
import { clearToasts, readToasts } from "../state/toasts";
import { useOpenPane } from "./useOpenPane";
import { NEW_TASK_PRESET_KEY, setNewTaskPreset } from "../state/newTaskPreset";

afterEach(() => { cleanup(); clearToasts(); localStorage.removeItem(NEW_TASK_PRESET_KEY); });

function blankPane() {
  const port = {
    agentPresets: vi.fn().mockRejectedValue(new Error("presets unavailable")),
    selectAgentComposition: vi.fn().mockResolvedValue(null),
    resume: vi.fn().mockResolvedValue(null),
  };
  const setActive = vi.fn();
  const hub = { open: vi.fn(), close: vi.fn() };
  const hook = renderHook(() => useOpenPane({
    hub: hub as unknown as HubPort,
    runtimes: [{ id: "blank", root: "/w" } as RuntimeView],
    panePorts: new Map([["blank", port as unknown as AgentPort]]),
    active: "other", reportsRef: { current: {} }, runsRef: { current: {} },
    setRuntimes: vi.fn(), setActive, setTakeover: vi.fn(), reloadPanes: vi.fn(async () => {}),
    sw: { path: "", pending: false, opening: () => false, begin: () => 1,
      current: () => true, landed: vi.fn(), end: vi.fn(), cancel: vi.fn() },
  }));
  return { port, setActive, hub, open: async (agentComposition?: string, sessionPath?: string) => {
    await act(async () => { await hook.result.current({ root: "/w", agentComposition, sessionPath }); });
  } };
}

it("keeps an existing blank pane's chosen mode without reading the catalog", async () => {
  const pane = blankPane();
  await pane.open("builtin:minimal");
  await pane.open();
  expect(pane.port.selectAgentComposition).toHaveBeenCalledExactlyOnceWith("builtin:minimal");
  expect(pane.port.agentPresets).not.toHaveBeenCalled();
  expect(pane.setActive).toHaveBeenLastCalledWith("blank");
  expect(pane.hub.open).not.toHaveBeenCalled();
});

it("applies a newly chosen default when New reuses an unused pane", async () => {
  const pane = blankPane();
  setNewTaskPreset("builtin:minimal");
  await pane.open();
  expect(pane.port.selectAgentComposition).toHaveBeenLastCalledWith("builtin:minimal");
  setNewTaskPreset("builtin:general");
  await pane.open();
  expect(pane.port.selectAgentComposition).toHaveBeenLastCalledWith("builtin:general");
  expect(pane.port.agentPresets).not.toHaveBeenCalled();
});

it("restores history into a blank pane without applying the new-task default", async () => {
  const pane = blankPane();
  setNewTaskPreset("builtin:minimal");
  await pane.open(undefined, "/w/history.jsonl");
  expect(pane.port.resume).toHaveBeenCalledWith("/w/history.jsonl");
  expect(pane.port.selectAgentComposition).not.toHaveBeenCalled();
});

it("still opens the pane and reports a refused explicit mode without rebuilding it", async () => {
  const pane = blankPane();
  pane.port.selectAgentComposition.mockRejectedValue(new HttpError(409, "locked", { code: "agent_composition.locked" }));
  await pane.open("builtin:minimal");
  expect(pane.setActive).toHaveBeenCalledWith("blank");
  expect(pane.hub.open).not.toHaveBeenCalled();
  expect(readToasts()).toHaveLength(1);
  expect(readToasts()[0].text).toContain("不能切换组合");
});
