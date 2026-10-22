// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { FeedbackBoard } from "./FeedbackBoard";
import { MockPort } from "../port/mock";
import { HttpError, type AgentPort } from "../port/port";
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
  port.feedbackBoard = vi.fn(port.feedbackBoard.bind(port));
  port.feedbackBoardMe = vi.fn(port.feedbackBoardMe.bind(port));
  port.joinFeedbackBoard = vi.fn(port.joinFeedbackBoard.bind(port));
  port.leaveFeedbackBoard = vi.fn(port.leaveFeedbackBoard.bind(port));
  return port;
}

describe("FeedbackBoard", () => {
  it("is not drawn at all where the service serves no board", async () => {
    const port = spied();
    port.feedbackBoard = vi.fn(async () => ({ available: false, view: "all" as const, release: null, entries: [] }));
    const { container } = render(<FeedbackBoard port={port} />);
    await waitFor(() => expect(port.feedbackBoard).toHaveBeenCalled());
    expect(container.querySelector(".fbk-board")).toBeNull();
  });

  it("lists all releases by name with no places, and a release with places where ties share one", async () => {
    const port = spied();
    render(<FeedbackBoard port={port} />);
    await screen.findByText("Sample Alpha");
    expect(document.querySelectorAll(".fbk-board-rank")).toHaveLength(0);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "版本" }), "v2.45.0");
    await waitFor(() => expect(document.querySelectorAll(".fbk-board-rank").length).toBeGreaterThan(0));
    const places = [...document.querySelectorAll(".fbk-board-rank")].map((n) => n.textContent);
    expect(places).toEqual(["1", "2", "2", "4"]);
    expect(port.feedbackBoard).toHaveBeenLastCalledWith("v2.45.0");
  });

  it("does not ask the account anything while signed out, and says to sign in", async () => {
    const port = spied();
    render(<FeedbackBoard port={port} />);
    expect(await screen.findByText("先在设置里登录账号，才能决定是否上榜。")).toBeTruthy();
    expect(port.feedbackBoardMe).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "加入贡献榜" })).toBeNull();
  });

  it("previews the exact name, joins only when pressed, and can leave again", async () => {
    signIn();
    const port = spied();
    render(<FeedbackBoard port={port} />);
    const field = await screen.findByRole("textbox", { name: "榜上显示的名字" });
    const join = screen.getByRole("button", { name: "加入贡献榜" }) as HTMLButtonElement;
    expect(join.disabled).toBe(true);
    await userEvent.type(field, "  Sample Person ");
    expect(screen.getByText("榜上将显示为：Sample Person")).toBeTruthy();
    expect(port.joinFeedbackBoard).not.toHaveBeenCalled();
    await userEvent.click(join);
    await waitFor(() => expect(port.joinFeedbackBoard).toHaveBeenCalledExactlyOnceWith("Sample Person"));
    await userEvent.click(await screen.findByRole("button", { name: "退出榜单" }));
    await waitFor(() => expect(port.leaveFeedbackBoard).toHaveBeenCalledTimes(1));
    expect(await screen.findByRole("textbox", { name: "榜上显示的名字" })).toBeTruthy();
  });

  it("says why a name was refused, from the refusal's code, and keeps the field", async () => {
    signIn();
    const port = spied();
    port.joinFeedbackBoard = vi.fn(async () => { throw new HttpError(400, "x", { code: "feedback.board_name_reserved" }); });
    render(<FeedbackBoard port={port} />);
    await userEvent.type(await screen.findByRole("textbox", { name: "榜上显示的名字" }), "Reasonix");
    await userEvent.click(screen.getByRole("button", { name: "加入贡献榜" }));
    expect((await screen.findByRole("alert")).textContent).toContain("保留");
    expect((screen.getByRole("textbox", { name: "榜上显示的名字" }) as HTMLInputElement).value).toBe("Reasonix");
  });

  it("says a maintainer removed the account instead of offering the join again", async () => {
    signIn();
    const port = spied();
    port.feedbackBoardMe = vi.fn(async () => ({ listed: false, displayName: null, delisted: true }));
    render(<FeedbackBoard port={port} />);
    expect(await screen.findByText("这个账号已被维护者从榜单移除，暂时不能再加入。")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "加入贡献榜" })).toBeNull();
  });
});
