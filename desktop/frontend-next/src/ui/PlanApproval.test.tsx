// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { PlanApproval } from "./PlanApproval";
import { MockPort } from "../port/mock";
import type { AgentPort } from "../port/port";

afterEach(cleanup);

const toggle = () => screen.findByRole("switch", { name: "自动批准计划" });

describe("the plan-approval switch", () => {
  it("starts off, says what it releases and what it leaves alone, and saves the opposite of what it shows", async () => {
    const p = new MockPort() as unknown as AgentPort;
    const save = vi.spyOn(p, "savePlanApproval");
    render(<PlanApproval port={p} />);
    const sw = await toggle();
    expect(sw.getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText(/工具批准、沙箱和拒绝规则照常生效/)).toBeTruthy();
    expect(screen.getByText(/项目配置不能替你打开/)).toBeTruthy();
    await userEvent.click(sw);
    expect(save).toHaveBeenCalledWith(true);
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("true"));
    await userEvent.click(sw);
    expect(save).toHaveBeenLastCalledWith(false);
  });

  it("keeps the switch where it was when the save is refused", async () => {
    const p = new MockPort() as unknown as AgentPort;
    p.savePlanApproval = async () => {
      throw new Error("disk full");
    };
    render(<PlanApproval port={p} />);
    const sw = await toggle();
    await userEvent.click(sw);
    await screen.findByText(/disk full/);
    expect(sw.getAttribute("aria-checked")).toBe("false");
  });

  it("says so when the setting cannot be read", async () => {
    const p = new MockPort() as unknown as AgentPort;
    p.planApproval = async () => {
      throw new Error("offline");
    };
    render(<PlanApproval port={p} />);
    expect(await screen.findByText("无法读取计划批准设置。")).toBeTruthy();
  });
});
