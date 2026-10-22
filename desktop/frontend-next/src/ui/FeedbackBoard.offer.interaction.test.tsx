// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { FeedbackBoard } from "./FeedbackBoard";
import { MockPort } from "../port/mock";
import type { AgentPort } from "../port/port";
import { boot, STORAGE } from "../i18n";

beforeEach(() => localStorage.setItem("reasonix.mock.account", "1"));
afterEach(() => {
  cleanup();
  localStorage.removeItem("reasonix.mock.account");
  localStorage.removeItem("rx-board-offer");
  localStorage.setItem(STORAGE, "zh");
  boot();
});

async function linked() {
  const port = new MockPort() as unknown as AgentPort;
  await port.linkFeedbackAccount();
  const env = port.feedbackEnv.bind(port);
  port.feedbackEnv = vi.fn(async (lang?: string) => ({ ...(await env(lang)), displayName: "Sample Alpha" }));
  port.joinFeedbackBoard = vi.fn(port.joinFeedbackBoard.bind(port));
  return port;
}

const offer = () => screen.queryByRole("group", { name: "要不要加入贡献榜" });

describe("the one-time offer to join the board on an install that was already linked", () => {
  it("is made once with the nickname filled in, and joins only on the click", async () => {
    const port = await linked();
    render(<FeedbackBoard port={port} />);
    await screen.findByText("要不要加入贡献榜？");
    await waitFor(() => expect((screen.getAllByRole("textbox", { name: "榜上显示的名字" })[0] as HTMLInputElement).value).toBe("Sample Alpha"));
    expect(port.joinFeedbackBoard).not.toHaveBeenCalled();
    await userEvent.click(screen.getAllByRole("button", { name: "加入贡献榜" })[0]);
    await waitFor(() => expect(port.joinFeedbackBoard).toHaveBeenCalledWith("Sample Alpha"));
    await waitFor(() => expect(offer()).toBeNull());
    expect(localStorage.getItem("rx-board-offer")).toBe("1");
  });

  it("can be closed, and is not made again after a reload", async () => {
    const port = await linked();
    const first = render(<FeedbackBoard port={port} />);
    await screen.findByText("要不要加入贡献榜？");
    await userEvent.click(screen.getByRole("button", { name: "不用了" }));
    expect(offer()).toBeNull();
    expect(port.joinFeedbackBoard).not.toHaveBeenCalled();
    first.unmount();
    render(<FeedbackBoard port={port} />);
    await screen.findByText("贡献榜");
    await waitFor(() => expect(screen.getByRole("button", { name: "加入贡献榜" })).toBeTruthy());
    expect(offer()).toBeNull();
  });

  it("is not made to someone who is signed out, not linked, already listed or removed", async () => {
    localStorage.removeItem("reasonix.mock.account");
    const out = render(<FeedbackBoard port={new MockPort() as unknown as AgentPort} />);
    await screen.findByText("贡献榜");
    expect(offer()).toBeNull();
    out.unmount();

    localStorage.setItem("reasonix.mock.account", "1");
    const unlinked = render(<FeedbackBoard port={new MockPort() as unknown as AgentPort} />);
    await screen.findByText("贡献榜");
    await waitFor(() => expect(screen.getAllByRole("textbox", { name: "榜上显示的名字" }).length).toBeGreaterThan(0));
    expect(offer()).toBeNull();
    unlinked.unmount();

    const listed = await linked();
    await listed.joinFeedbackBoard("Sample Alpha");
    render(<FeedbackBoard port={listed} />);
    await screen.findByText(/榜上显示为/);
    expect(offer()).toBeNull();
  });
});
