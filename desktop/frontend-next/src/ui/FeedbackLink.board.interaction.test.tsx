// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { FeedbackLink } from "./FeedbackLink";
import { MockPort } from "../port/mock";
import { HttpError, type AgentPort } from "../port/port";
import { FEEDBACK_CODE } from "../port/feedback";
import { boot, STORAGE } from "../i18n";

beforeEach(() => localStorage.setItem("reasonix.mock.account", "1"));
afterEach(() => {
  cleanup();
  localStorage.removeItem("reasonix.mock.account");
  localStorage.removeItem("rx-board-offer");
  localStorage.setItem(STORAGE, "zh");
  boot();
});

function spied(nickname = "Sample Alpha") {
  const port = new MockPort() as unknown as AgentPort;
  const env = port.feedbackEnv.bind(port);
  port.feedbackEnv = vi.fn(async (lang?: string) => ({ ...(await env(lang)), displayName: nickname }));
  port.linkFeedbackAccount = vi.fn(port.linkFeedbackAccount.bind(port));
  port.joinFeedbackBoard = vi.fn(port.joinFeedbackBoard.bind(port));
  return port;
}

const open = async () => userEvent.click(await screen.findByRole("button", { name: "把这台电脑关联到我的账号" }));
const choice = () => screen.findByRole("checkbox", { name: "加入贡献榜" }) as Promise<HTMLInputElement>;
const nameBox = () => screen.getByRole("textbox", { name: "榜上显示的名字" }) as HTMLInputElement;
const confirm = () => screen.getByRole("button", { name: "确认" }) as HTMLButtonElement;

describe("joining the contribution board while linking", () => {
  it("offers the board already ticked with the feedback nickname, and links nothing until confirmed", async () => {
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    await open();
    expect((await choice()).checked).toBe(true);
    await waitFor(() => expect(nameBox().value).toBe("Sample Alpha"));
    expect(screen.getByText("榜上将显示为：Sample Alpha")).toBeTruthy();
    expect(port.linkFeedbackAccount).not.toHaveBeenCalled();
    expect(port.joinFeedbackBoard).not.toHaveBeenCalled();
  });

  it("links and then joins under the shown name when confirmed", async () => {
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    await open();
    await choice();
    await waitFor(() => expect(nameBox().value).toBe("Sample Alpha"));
    await userEvent.clear(nameBox());
    await userEvent.type(nameBox(), "Fresh Name");
    await userEvent.click(confirm());
    await screen.findByText(/这台电脑已关联/);
    expect(port.linkFeedbackAccount).toHaveBeenCalledTimes(1);
    expect(port.joinFeedbackBoard).toHaveBeenCalledWith("Fresh Name");
  });

  it("does not join when the box is unticked", async () => {
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    await open();
    await userEvent.click(await choice());
    await userEvent.click(confirm());
    await screen.findByText(/这台电脑已关联/);
    expect(port.linkFeedbackAccount).toHaveBeenCalledTimes(1);
    expect(port.joinFeedbackBoard).not.toHaveBeenCalled();
  });

  it("leaves the name empty when the nickname is not one the board takes, and asks for one", async () => {
    const port = spied("a.b@c");
    render(<FeedbackLink port={port} hasReport />);
    await open();
    await choice();
    expect(nameBox().value).toBe("");
    expect(confirm().disabled).toBe(true);
    expect(screen.getByText("请填写一个名字，或取消勾选就不加入。")).toBeTruthy();
  });

  it.each([["a", "名字至少 2 个字符。"], ["x".repeat(31), "名字最多 30 个字符。"], ["me@home", "名字只能用字母、数字、空格、下划线或连字符，不能含 @ 或句点。"], ["a.b", "名字只能用字母、数字、空格、下划线或连字符，不能含 @ 或句点。"]])("refuses the name %j before asking the service", async (typed, words) => {
    const port = spied("");
    render(<FeedbackLink port={port} hasReport />);
    await open();
    await choice();
    await userEvent.type(nameBox(), typed);
    expect((await screen.findByRole("alert")).textContent).toBe(words);
    expect(confirm().disabled).toBe(true);
    expect(port.linkFeedbackAccount).not.toHaveBeenCalled();
  });

  it("says so when linking fails and does not try the board", async () => {
    const port = spied();
    port.linkFeedbackAccount = vi.fn(async () => { throw new HttpError(409, "link_conflict", { code: FEEDBACK_CODE.linkConflict, error: "x" }); });
    render(<FeedbackLink port={port} hasReport />);
    await open();
    await choice();
    await waitFor(() => expect(nameBox().value).toBe("Sample Alpha"));
    await userEvent.click(confirm());
    expect((await screen.findByRole("alert")).textContent).toContain("已经关联到另一个账号");
    expect(port.joinFeedbackBoard).not.toHaveBeenCalled();
  });

  it("keeps the link and says why when the service refuses the name", async () => {
    const port = spied();
    const changed = vi.fn();
    port.joinFeedbackBoard = vi.fn(async () => { throw new HttpError(409, "taken", { code: "feedback.board_name_taken", error: "x" }); });
    render(<FeedbackLink port={port} hasReport onBoardChanged={changed} />);
    await open();
    await choice();
    await waitFor(() => expect(nameBox().value).toBe("Sample Alpha"));
    await userEvent.click(confirm());
    await screen.findByText(/这台电脑已关联/);
    const note = await screen.findByRole("alert");
    expect(note.textContent).toContain("已关联，但没能加入贡献榜");
    expect(note.textContent).toContain("已经有人用这个名字了");
    expect(changed).toHaveBeenCalled();
  });

  it("cancelling closes the step without linking", async () => {
    const port = spied();
    render(<FeedbackLink port={port} hasReport />);
    await open();
    await choice();
    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("checkbox", { name: "加入贡献榜" })).toBeNull();
    expect(port.linkFeedbackAccount).not.toHaveBeenCalled();
  });
});
