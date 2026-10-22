// @vitest-environment jsdom
import { memo } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { App } from "./App";
import { MockHub } from "../port/mock_hub";
import type { RuntimeView, TreeWorkspace } from "../port/hub";
import type { AgentPort } from "../port/port";

const renders = vi.hoisted(() => new Map<string, number>());
const shown = vi.hoisted(() => new Map<string, { visible: boolean; focus: () => void }>());

vi.mock("./Pane", async (original) => {
  const real = await original<typeof import("./Pane")>();
  const Counting = memo(function CountingPane(props: { rt: { id: string }; visible: boolean; onFocus: () => void }) {
    shown.set(props.rt.id, { visible: props.visible, focus: props.onFocus });
    renders.set(props.rt.id, (renders.get(props.rt.id) ?? 0) + 1);
    return <section className="pane" />;
  });
  return { ...real, Pane: Counting };
});

afterEach(cleanup);

const PANES = 6;

const views: RuntimeView[] = Array.from({ length: PANES }, (_, i) => ({
  id: `r${i + 1}`, base: i === 0 ? "" : `/rt/r${i + 1}`, root: "~/projects/DeepSeek-Reasonix", name: "DeepSeek-Reasonix",
  sessionPath: `/sessions/pane-${i + 1}.jsonl`,
}));

class SixPaneHub extends MockHub {
  open(req: { root?: string; sessionPath?: string }) {
    const held = views.find((v) => v.sessionPath === req.sessionPath);
    return held ? Promise.resolve(held) : super.open(req);
  }
  portFor(rt: RuntimeView): AgentPort {
    const port = super.portFor(rt);
    port.providerSetup = async () => null;
    port.welcomeSeen = async () => true;
    return port;
  }
  runtimes(): Promise<RuntimeView[]> {
    return Promise.resolve([...views]);
  }
  tree(): Promise<TreeWorkspace[]> {
    return Promise.resolve([{
      root: "~/projects/DeepSeek-Reasonix", name: "DeepSeek-Reasonix", open: true,
      sessions: Array.from({ length: PANES }, (_, i) => ({ path: `/sessions/pane-${i + 1}.jsonl`, name: `pane-${i + 1}`, title: `会话 ${i + 1}`, turns: 3 })),
    }]);
  }
}

// A hidden pane holds a whole transcript. Switching tabs changes which pane is
// active, so only the one that stops being active and the one that becomes active
// have anything new to draw; every other pane must keep what it already drew.
it("re-renders only the two panes a tab switch touches", async () => {
  const { container } = render(<App hub={new SixPaneHub()} />);
  const rows = await waitFor(() => {
    const found = [...container.querySelectorAll<HTMLElement>(".sessrow")];
    expect(found.length).toBeGreaterThanOrEqual(PANES);
    return found;
  });
  await new Promise((r) => setTimeout(r, 300));
  renders.clear();
  await userEvent.click(rows[3]);
  await new Promise((r) => setTimeout(r, 200));
  const touched = [...renders.entries()].filter(([, n]) => n > 0).map(([id]) => id);
  expect(touched.length).toBeLessThanOrEqual(2);
  expect(Math.max(0, ...renders.values())).toBeLessThanOrEqual(2);
});

// Stable callbacks must still reach the latest state: the pane that was asked for
// is the one drawn as visible, and the others are not.
it("still makes the chosen pane the visible one, and its focus callback works", async () => {
  const { container } = render(<App hub={new SixPaneHub()} />);
  const rows = await waitFor(() => {
    const found = [...container.querySelectorAll<HTMLElement>(".sessrow")];
    expect(found.length).toBeGreaterThanOrEqual(PANES);
    return found;
  });
  await userEvent.click(rows.find((r) => r.textContent?.includes("会话 5"))!);
  await waitFor(() => expect([...shown.entries()].filter(([, p]) => p.visible).map(([id]) => id)).toEqual(["r5"]));
  shown.get("r2")!.focus();
  await waitFor(() => expect([...shown.entries()].filter(([, p]) => p.visible).map(([id]) => id)).toEqual(["r2"]));
});
