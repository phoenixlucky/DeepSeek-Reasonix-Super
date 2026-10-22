// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import "./testkit";
import { App } from "./App";
import { MockHub } from "../port/mock_hub";
import { host } from "../port/host";
import { MockPort } from "../port/mock";
import { boot, STORAGE } from "../i18n";
import type { AgentPort } from "../port/port";

beforeEach(() => {
  vi.spyOn(host(), "inShell").mockReturnValue(true);
  localStorage.setItem(STORAGE, "zh");
  boot();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

async function mount() {
  const hub = new MockHub();
  const port = new MockPort() as unknown as AgentPort;
  port.providerSetup = async () => null;
  port.welcomeSeen = async () => true;
  hub.portFor = () => port;
  hub.machinePort = () => port;
  render(<App hub={hub} />);
  return waitFor(() => {
    const el = document.querySelector<HTMLTextAreaElement>(".pane textarea");
    if (!el) throw new Error("no composer yet");
    return el;
  });
}

const chord = (target: Element, digit: string) =>
  fireEvent.keyDown(target, { key: digit, code: `Digit${digit}`, ctrlKey: true });
const selected = () => document.querySelector('.chrome [data-action="pane.view"][aria-pressed="true"]')?.getAttribute("data-value") ?? "flow";

it("Mod+3 opens the workbench and Mod+1 returns to the conversation, even from the composer", async () => {
  const box = await mount();
  expect(selected()).toBe("flow");
  chord(box, "3");
  await waitFor(() => expect(selected()).toBe("browser"));
  chord(box, "1");
  await waitFor(() => expect(selected()).toBe("flow"));
});

it("Mod+2 does nothing while the conversation has no rows, because the view is not drawn", async () => {
  const box = await mount();
  chord(box, "2");
  await new Promise((r) => setTimeout(r, 50));
  expect(selected()).toBe("flow");
});

it("the command palette lists the three views", async () => {
  const box = await mount();
  fireEvent.keyDown(box, { key: "k", code: "KeyK", ctrlKey: true });
  // The palette is a lazy chunk; under a loaded runner the default second is not enough.
  await waitFor(() => expect(document.body.textContent).toContain("显示工作台"), { timeout: 10_000 });
  expect(document.body.textContent).toContain("显示对话");
});
