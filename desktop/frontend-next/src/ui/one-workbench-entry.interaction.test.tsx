// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { App } from "./App";
import { MockHub } from "../port/mock_hub";

afterEach(cleanup);

function readyHub() {
  const hub = new MockHub();
  const build = hub.portFor.bind(hub);
  const opened = vi.fn();
  hub.portFor = (rt) => {
    const port = build(rt);
    port.providerSetup = async () => null;
    port.welcomeSeen = async () => true;
    const open = port.browserOpen?.bind(port);
    port.browserOpen = ((...args: Parameters<typeof port.browserOpen>) => { opened(...args); return open!(...args); }) as typeof port.browserOpen;
    return port;
  };
  return { hub, opened };
}

describe("the workbench has one way in", () => {
  it("has no separate browser button in the title bar", async () => {
    render(<App hub={readyHub().hub} />);
    await screen.findByRole("button", { name: "工作台" });
    expect(document.querySelectorAll('[data-action="pane.view"][data-value="browser"]')).toHaveLength(1);
    expect(document.querySelector(".chrome .browser-action")).toBeNull();
    expect(document.querySelector('[data-action="browser.open"]')).toBeNull();
  });

  it("opens on its start page when nothing is running, and does not open a blank browser page", async () => {
    const user = userEvent.setup();
    const { hub, opened } = readyHub();
    render(<App hub={hub} />);
    await user.click(await screen.findByRole("button", { name: "工作台" }));

    await waitFor(() => expect(document.querySelector(".workbench-launcher")).not.toBeNull());
    expect(opened).not.toHaveBeenCalled();
  });
});
