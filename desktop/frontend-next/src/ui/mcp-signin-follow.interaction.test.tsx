// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { ServerRow } from "./ServerRow";
import { HttpError } from "../port/http_error";
import type { AgentPort, McpEntry } from "../port/port";

afterEach(cleanup);

const signedOut = (over: Partial<McpEntry> = {}): McpEntry => ({
  name: "figma", state: "failed", enabled: true, transport: "http", tools: 0, httpStatus: 401, ...over,
});
const withheld = (over: Partial<McpEntry> = {}): McpEntry => ({
  name: "notes", state: "ready", enabled: true, transport: "stdio", tools: 1,
  held: [{ name: "mcp__notes__search", class: "changed" }], heldDigest: "digest-1", ...over,
});

function draw(m: McpEntry, port: Partial<AgentPort> = {}, live = true) {
  const onDone = vi.fn();
  render(<ServerRow m={m} port={port as AgentPort} onDone={onDone} root="/w" live={live} />);
  return { onDone };
}

describe("signing in to a server that answered 401", () => {
  it("offers sign-in on that row, beside the retry", () => {
    draw(signedOut());
    expect(screen.getByRole("button", { name: "登录" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "重连" })).toBeTruthy();
  });

  it("offers it nowhere else: not a healthy server, not another failure, not a row that cannot be driven", () => {
    draw(signedOut({ state: "ready", httpStatus: undefined }));
    expect(screen.queryByRole("button", { name: "登录" })).toBeNull();
    cleanup();
    draw(signedOut({ httpStatus: 500 }));
    expect(screen.queryByRole("button", { name: "登录" })).toBeNull();
    cleanup();
    draw(signedOut(), {}, false);
    expect(screen.queryByRole("button", { name: "登录" })).toBeNull();
  });

  it("waits for the browser sign-in and says so until the request returns", async () => {
    let finish!: (v: { name: string; state: string; tools: number }) => void;
    const signinMcp = vi.fn(() => new Promise<{ name: string; state: string; tools: number }>((r) => { finish = r; }));
    const { onDone } = draw(signedOut(), { signinMcp });
    await userEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(signinMcp).toHaveBeenCalledExactlyOnceWith("figma");
    expect(await screen.findByText("等待浏览器登录…")).toBeTruthy();
    expect((screen.getByRole("button", { name: /等待浏览器登录/ }) as HTMLButtonElement).disabled).toBe(true);
    finish({ name: "figma", state: "ready", tools: 3 });
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(screen.queryByText("等待浏览器登录…")).toBeNull();
  });

  it("shows the kernel's refusal, from its code, when the person declines or it times out", async () => {
    const signinMcp = vi.fn(async () => { throw new HttpError(409, "x", { code: "mcp.signin_declined", error: "the sign-in was declined" }); });
    draw(signedOut(), { signinMcp });
    await userEvent.click(screen.getByRole("button", { name: "登录" }));
    expect((await screen.findByRole("alert")).textContent).toBeTruthy();
    expect((screen.getByRole("button", { name: "登录" }) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("granting a server the access it asked for", () => {
  const asking = (over: Partial<McpEntry> = {}): McpEntry => ({
    name: "figma", state: "ready", enabled: true, transport: "http", tools: 2, scopeUpgrade: ["files:write", "admin"], ...over,
  });

  it("shows the server's scope names as written, beside a button that asks", () => {
    draw(asking());
    expect(screen.getByText("files:write admin")).toBeTruthy();
    expect(screen.getByRole("button", { name: "授权更多权限" })).toBeTruthy();
  });

  it("offers nothing for a server that asked for nothing, or a row that cannot be driven", () => {
    draw(asking({ scopeUpgrade: undefined }));
    expect(screen.queryByRole("button", { name: "授权更多权限" })).toBeNull();
    cleanup();
    draw(asking(), {}, false);
    expect(screen.queryByRole("button", { name: "授权更多权限" })).toBeNull();
  });

  it("waits for the browser, and shows the kernel's refusal by its code", async () => {
    const upgradeMcp = vi.fn(async () => { throw new HttpError(409, "x", { code: "mcp.signin_declined", error: "declined" }); });
    draw(asking(), { upgradeMcp });
    await userEvent.click(screen.getByRole("button", { name: "授权更多权限" }));
    expect(upgradeMcp).toHaveBeenCalledExactlyOnceWith("figma");
    expect((await screen.findByRole("alert")).textContent).toBeTruthy();
    expect((screen.getByRole("button", { name: "授权更多权限" }) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("following a server's changes", () => {
  it("approves what was shown, and only then asks to follow from now on", async () => {
    const order: string[] = [];
    const trustMcp = vi.fn(async () => { order.push("trust"); return { state: "ready", tools: 2 }; });
    const followMcp = vi.fn(async () => { order.push("follow"); return { name: "notes", follow: true, tools: 2 }; });
    draw(withheld(), { trustMcp, followMcp });
    await userEvent.click(screen.getByRole("button", { name: "接受并以后都自动接受这个服务器的改动" }));
    await waitFor(() => expect(followMcp).toHaveBeenCalled());
    expect(trustMcp).toHaveBeenCalledExactlyOnceWith("notes", "digest-1");
    expect(followMcp).toHaveBeenCalledExactlyOnceWith("notes", true);
    expect(order).toEqual(["trust", "follow"]);
  });

  it("does not follow when the approval itself was refused", async () => {
    const trustMcp = vi.fn(async () => ({ state: "failed", error: "the held MCP tool definitions changed since they were shown" }));
    const followMcp = vi.fn();
    draw(withheld(), { trustMcp, followMcp });
    await userEvent.click(screen.getByRole("button", { name: "接受并以后都自动接受这个服务器的改动" }));
    await waitFor(() => expect(trustMcp).toHaveBeenCalled());
    expect(followMcp).not.toHaveBeenCalled();
    expect(await screen.findByText(/changed since they were shown/)).toBeTruthy();
  });

  it("keeps the plain approval beside it, which follows nothing", async () => {
    const trustMcp = vi.fn(async () => ({ state: "ready", tools: 2 }));
    const followMcp = vi.fn();
    draw(withheld(), { trustMcp, followMcp });
    await userEvent.click(screen.getByRole("button", { name: "批准并重新连接" }));
    await waitFor(() => expect(trustMcp).toHaveBeenCalled());
    expect(followMcp).not.toHaveBeenCalled();
  });

  it("offers to stop following while it is followed, and only then", async () => {
    draw(withheld({ held: undefined, heldDigest: undefined }));
    expect(screen.queryByRole("button", { name: "取消跟随" })).toBeNull();
    cleanup();
    const followMcp = vi.fn(async () => ({ name: "notes", follow: false, tools: 1 }));
    draw(withheld({ held: undefined, heldDigest: undefined, followsChanges: true }), { followMcp });
    await userEvent.click(screen.getByRole("button", { name: "取消跟随" }));
    expect(followMcp).toHaveBeenCalledExactlyOnceWith("notes", false);
  });

  it("says a followed server's changes are accepted by themselves", () => {
    draw(withheld({ held: undefined, heldDigest: undefined, followsChanges: true }));
    expect(screen.getByText(/这个服务器的改动会自动接受/)).toBeTruthy();
  });
});
