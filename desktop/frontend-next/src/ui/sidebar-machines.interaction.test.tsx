// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { MockHub } from "../port/mock_hub";
import type { TreeWorkspace } from "../port/hub";
import { Sidebar } from "./Sidebar";
import { NO_PAGING } from "./treewindow";

afterEach(cleanup);

const tree: TreeWorkspace[] = [{ root: "/work/alpha", name: "alpha", remembered: true, sessions: [{ path: "/work/alpha/a.jsonl", name: "a", title: "first" }] }];

function draw(navShown: boolean, onSettings = vi.fn()) {
  const noop = vi.fn();
  render(
    <Sidebar
      onFiles={noop} onView={noop} hub={new MockHub()} collapsed={false} navShown={navShown} paging={NO_PAGING} tree={tree} treeRead runtimes={[]} runs={{}} active=""
      activeWorkspace={tree[0]} liveIds={() => []} pinned={new Set()} onPin={noop} remotes={null} remoteTrees={{}}
      reloadRemotes={async () => {}} reloadRemoteTrees={async () => {}} readRemoteTree={async () => {}} reloadTree={async () => {}}
      adder={{ busy: false, add: noop, addPath: noop } as never} onOpen={async () => {}} onOpenRemote={async () => {}} onFocusPane={noop} onClosePanes={async () => {}}
      onPause={noop} onArchive={async () => {}} onRename={noop} onCollapse={noop} account={null} accountUnread="" wallet=""
      onSettings={onSettings} onFeedback={noop} feedbackUnread={0} onFind={noop} onError={noop}
    />,
  );
}

const entry = () => screen.queryByRole("button", { name: /连接与机器/ });

it("does not repeat the Remote entry of the icon rail while the rail is shown", () => {
  draw(true);
  expect(entry()).toBeNull();
});

it("keeps the way into the remote settings when there is no icon rail, and it opens that section", async () => {
  const onSettings = vi.fn();
  draw(false, onSettings);
  await userEvent.click(entry()!);
  expect(onSettings).toHaveBeenCalledWith("remote");
});
