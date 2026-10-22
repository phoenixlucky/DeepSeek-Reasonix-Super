// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { FeedbackLink } from "./FeedbackLink";
import { MockPort } from "../port/mock";
import { HttpError, type AgentPort } from "../port/port";
import { FEEDBACK_CODE } from "../port/feedback";
import { boot, STORAGE } from "../i18n";

afterEach(() => {
  cleanup();
  localStorage.removeItem("reasonix.mock.account");
  localStorage.setItem(STORAGE, "zh");
  boot();
});

const signIn = () => localStorage.setItem("reasonix.mock.account", "1");

function spied() {
  const port = new MockPort() as unknown as AgentPort;
  port.feedbackLinks = vi.fn(port.feedbackLinks.bind(port));
  port.linkFeedbackAccount = vi.fn(port.linkFeedbackAccount.bind(port));
  port.unlinkFeedbackAccount = vi.fn(port.unlinkFeedbackAccount.bind(port));
  port.unlinkFeedbackInstall = vi.fn(port.unlinkFeedbackInstall.bind(port));
  return port;
}

describe("FeedbackLink", () => {
  it("is inert while signed out and never asks the service about links", async () => {
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    const link = await screen.findByRole("button", { name: "把这台电脑关联到我的账号" });
    expect((link as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("先在设置里登录账号，才能关联。")).toBeTruthy();
    expect(port.feedbackLinks).not.toHaveBeenCalled();
    await userEvent.click(link);
    expect(port.linkFeedbackAccount).not.toHaveBeenCalled();
  });

  it("waits for a first report before offering the link", async () => {
    signIn();
    const port = spied();
    render(<FeedbackLink port={port} hasReport={false} />);
    expect((await screen.findByRole("button", { name: "把这台电脑关联到我的账号" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("发出第一条反馈之后才能关联这台电脑。")).toBeTruthy();
  });

  it("links only on the click, then shows this computer as linked and unlinks it again", async () => {
    signIn();
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    const link = await screen.findByRole("button", { name: "把这台电脑关联到我的账号" });
    expect(port.linkFeedbackAccount).not.toHaveBeenCalled();
    await userEvent.click(link);
    expect(port.linkFeedbackAccount).not.toHaveBeenCalled();
    await userEvent.click(await screen.findByRole("checkbox", { name: "加入贡献榜" }));
    await userEvent.click(screen.getByRole("button", { name: "确认" }));
    await screen.findByText(/这台电脑已关联/);
    expect(port.linkFeedbackAccount).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "解除关联" }));
    await screen.findByRole("button", { name: "把这台电脑关联到我的账号" });
    expect(port.unlinkFeedbackAccount).toHaveBeenCalledTimes(1);
  });

  it("lists the account's other computers and unlinks one by its short id", async () => {
    signIn();
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    await screen.findByText("ba9876543210");
    await userEvent.click(screen.getByRole("button", { name: "解除" }));
    await waitFor(() => expect(port.unlinkFeedbackInstall).toHaveBeenCalledWith("ba9876543210"));
    await waitFor(() => expect(screen.queryByText("ba9876543210")).toBeNull());
  });

  it("says why a refusal happened in its own words", async () => {
    signIn();
    const port = spied();
    port.linkFeedbackAccount = vi.fn(async () => {
      throw new HttpError(409, "link_conflict", { code: FEEDBACK_CODE.linkConflict, error: "x" });
    });
    render(<FeedbackLink port={port} hasReport />);
    await userEvent.click(await screen.findByRole("button", { name: "把这台电脑关联到我的账号" }));
    await userEvent.click(await screen.findByRole("checkbox", { name: "加入贡献榜" }));
    await userEvent.click(screen.getByRole("button", { name: "确认" }));
    expect((await screen.findByRole("alert")).textContent).toContain("已经关联到另一个账号");
  });
});
