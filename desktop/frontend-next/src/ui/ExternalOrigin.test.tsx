// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { ExternalOrigin } from "./ExternalOrigin";
import { MockPort } from "../port/mock";
import type { AgentPort } from "../port/port";

afterEach(cleanup);

const toggle = () => screen.findByRole("switch", { name: "询问来自外部的文字" });

describe("the ask-on-external-text switch", () => {
  it("starts off, says what it covers and what it does not, and saves the opposite of what it shows", async () => {
    const p = new MockPort() as unknown as AgentPort;
    const save = vi.spyOn(p, "savePermissions");
    const changed = vi.fn();
    render(<ExternalOrigin port={p} onChanged={changed} />);
    const sw = await toggle();
    expect(sw.getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText(/已授权的 MCP 服务器、改写过或编码过的文字不在其内/)).toBeTruthy();
    expect(screen.getByText(/无人可问时（无头 YOLO），这类调用会被拒绝而不是执行/)).toBeTruthy();
    await userEvent.click(sw);
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ askOnExternalOrigin: true }));
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("true"));
    expect(changed).toHaveBeenCalledTimes(1);
    await userEvent.click(sw);
    expect(save).toHaveBeenLastCalledWith(expect.objectContaining({ askOnExternalOrigin: false }));
  });

  it("saves the rule lists as the kernel holds them now, not as they were when the page loaded", async () => {
    const p = new MockPort() as unknown as AgentPort;
    render(<ExternalOrigin port={p} onChanged={() => {}} />);
    const sw = await toggle();
    await p.savePermissions({ mode: "ask", deny: ["bash(rm *)"], ask: [], allow: ["read_file"] });
    const save = vi.spyOn(p, "savePermissions");
    await userEvent.click(sw);
    expect(save).toHaveBeenCalledWith({ mode: "ask", deny: ["bash(rm *)"], ask: [], allow: ["read_file"], askOnExternalOrigin: true });
  });

  it("a rules save that does not mention the switch leaves it as it is", async () => {
    const p = new MockPort() as unknown as AgentPort;
    const on = await p.savePermissions({ mode: "ask", deny: [], ask: [], allow: [], askOnExternalOrigin: true });
    expect(on.askOnExternalOrigin).toBe(true);
    const after = await p.savePermissions({ mode: "ask", deny: [], ask: ["bash"], allow: [] });
    expect(after.askOnExternalOrigin).toBe(true);
  });

  it("keeps the switch where it was and says why when the save is refused", async () => {
    const p = new MockPort() as unknown as AgentPort;
    p.savePermissions = async () => {
      throw new Error("disk full");
    };
    render(<ExternalOrigin port={p} onChanged={() => {}} />);
    const sw = await toggle();
    await userEvent.click(sw);
    await screen.findByText(/disk full/);
    expect(sw.getAttribute("aria-checked")).toBe("false");
  });
});
