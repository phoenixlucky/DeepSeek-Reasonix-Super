// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import "./testkit";
import { Pane } from "./Pane";
import { MockPort } from "../port/mock";
import type { AgentPort } from "../port/port";
import type { RuntimeView } from "../port/hub";
import type { PaneReport } from "./panetypes";

afterEach(cleanup);

const rt = { id: "p1", root: "/w", name: "w" } as RuntimeView;
const ASKING = { defaulted: true, writesConfined: true, trust: "", trustable: true } as const;

async function open(active: boolean, approvalDefault: unknown = ASKING) {
  const port = new MockPort();
  const read = port.status.bind(port);
  vi.spyOn(port, "status").mockImplementation(async () => ({ ...(await read()), toolApprovalMode: "ask", approvalDefault }) as never);
  const reports: PaneReport[] = [];
  const props = {
    port: port as AgentPort, rt, title: "w", active, visible: active, sideHost: null, side: false, onFocus: () => {},
    onReport: (_id: string, r: PaneReport) => void reports.push(r), onSessionChanged: () => {}, pulse: 0, findPulse: 0, onSettings: () => {},
    needsProject: false, onOpenProject: () => {}, onKeepHere: () => {}, theme: "dark", dockW: 560, dockMax: 880, onDockW: () => {},
  };
  const view = render(<Pane {...props} />);
  await screen.findByRole("group", { name: "是否信任此文件夹" }).catch(() => null);
  return { reports, last: () => reports[reports.length - 1], show: (next: boolean) => act(() => void view.rerender(<Pane {...props} active={next} visible={next} />)) };
}

describe("a pane that is waiting for the person to decide whether to trust its folder", () => {
  it("reports itself as waiting while it is in the background, so the sidebar can say so", async () => {
    const p = await open(false);
    await waitFor(() => expect(p.last()?.run).toBe("halt"));
    expect(p.last()?.live).toBe(true);
  });

  it("does not report waiting for the pane the person is looking at, which shows the card itself", async () => {
    const p = await open(true);
    await waitFor(() => expect(p.last()?.status?.approvalDefault).toBeTruthy());
    expect(p.last()?.run).toBe("idle");
    expect(p.last()?.live).toBe(false);
  });

  it("stops reporting waiting when the person switches to the pane", async () => {
    const p = await open(false);
    await waitFor(() => expect(p.last()?.run).toBe("halt"));
    await p.show(true);
    await waitFor(() => expect(p.last()?.run).toBe("idle"));
  });

  it("reports nothing special once the folder has been decided", async () => {
    const p = await open(false, { ...ASKING, trust: "trusted" });
    await waitFor(() => expect(p.last()?.status?.approvalDefault).toBeTruthy());
    expect(p.last()?.run).toBe("idle");
    expect(p.last()?.live).toBe(false);
  });
});
