// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Policy } from "./Policy";
import studioCss from "../styles/studio.css?raw";
import { MockPort } from "../port/mock";
import type { AgentPort, ApprovalMode, SessionStatus } from "../port/port";

afterEach(cleanup);

const status = (mode: ApprovalMode = "ask") =>
  ({ preset: "balanced", effort: "auto", toolApprovalMode: mode } as SessionStatus);

function draw(mode: ApprovalMode = "ask", onBoundary?: () => void) {
  const onChanged = vi.fn();
  const port = new MockPort() as unknown as AgentPort;
  const view = render(<Policy port={port} status={status(mode)} onChanged={onChanged} onBoundary={onBoundary} />);
  return { ...view, port, onChanged };
}

describe("execution permission control", () => {
  it("names the committed permission directly", () => {
    const { container } = draw();
    expect(container.querySelector('[data-action="chrome.policy"]')?.getAttribute("aria-label")).toBe("执行权限：询问");
    expect(container.querySelector(".policy")?.hasAttribute("data-quiet")).toBe(true);
  });

  it("keeps the dangerous state visible on the closed trigger", () => {
    const { container } = draw("yolo");
    expect(container.querySelector(".polrisk")?.textContent).toContain("全部放行");
    expect(container.querySelector(".polwarn svg")).not.toBeNull();
  });

  it("draws its chevron with the icon every other picker uses", () => {
    const { container } = draw("yolo");
    const chevron = container.querySelector(".studio-control-chevron");
    expect(chevron?.tagName.toLowerCase()).toBe("svg");
    expect(chevron?.querySelector("path")?.getAttribute("d")).toBe("m6 9 6 6 6-6");
  });

  it.each(["light", "dark"])("draws the warning icon, label and chevron in one colour in the %s theme", (theme) => {
    const sheet = document.createElement("style");
    sheet.textContent = studioCss;
    document.head.append(sheet);
    document.documentElement.dataset.theme = theme;
    try {
      const { container } = draw("yolo");
      container.classList.add("turntools");
      const colour = (selector: string) => getComputedStyle(container.querySelector(selector)!).color;
      expect(colour(".policy .ic")).not.toBe("");
      expect(colour(".policy .ic")).toBe(colour(".policy .lb"));
      expect(colour(".policy .studio-control-chevron")).toBe(colour(".policy .lb"));
    } finally {
      sheet.remove();
      delete document.documentElement.dataset.theme;
    }
  });

  it("contains permission choices only", async () => {
    draw();
    await userEvent.click(screen.getByRole("button", { name: "执行权限：询问" }));
    const group = screen.getByRole("group", { name: "执行权限" });
    expect(group.textContent).toMatch(/不询问.*询问.*自动批准.*全部放行/s);
    expect(group.textContent).not.toMatch(/高级执行设置|均衡|交付|思考强度/);
  });

  it("opens the sandbox boundary from the same layer", async () => {
    const onBoundary = vi.fn();
    draw("ask", onBoundary);
    await userEvent.click(screen.getByRole("button", { name: "执行权限：询问" }));
    await userEvent.click(screen.getByRole("button", { name: /沙盒与运行边界/ }));
    expect(onBoundary).toHaveBeenCalledTimes(1);
  });

  it("draws nothing before the kernel has answered", () => {
    const { container } = render(
      <Policy port={new MockPort() as unknown as AgentPort} status={null} onChanged={() => {}} />,
    );
    expect(container.querySelector(".policy")).toBeNull();
  });
});
