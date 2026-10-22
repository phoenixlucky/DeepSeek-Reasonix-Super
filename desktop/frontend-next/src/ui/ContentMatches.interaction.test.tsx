// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { HistoryHit, TreeWorkspace } from "../port/hub";
import { ContentMatches } from "./ContentMatches";
import { RailSearch } from "./railsearch";

const tree = [{ root: "/w/a", name: "a", sessions: [{ path: "/w/a/s1.jsonl", name: "s1", title: "Rotate keys" }] }] as unknown as TreeWorkspace[];
const hit: HistoryHit = { path: "/w/a/s1.jsonl", id: "s1", snippet: "rotate the zebra credentials", role: "user", messageIndex: 0, score: 1 };

function setup(search: (q: string) => Promise<HistoryHit[]>, workspaces = tree, onReference?: (id: string, title: string) => void) {
  const onOpen = vi.fn(() => Promise.resolve());
  render(
    <RailSearch>
      <ContentMatches hub={{ searchHistory: search }} tree={workspaces} onOpen={onOpen} onError={() => {}} onReference={onReference} />
    </RailSearch>,
  );
  return onOpen;
}

describe("ContentMatches", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("asks only after the reader pauses on two or more characters, then opens the match in its workspace", async () => {
    const search = vi.fn(() => Promise.resolve([hit]));
    const onOpen = setup(search);
    const box = screen.getByRole("searchbox");

    fireEvent.change(box, { target: { value: "z" } });
    await act(async () => void vi.advanceTimersByTime(1000));
    expect(search).not.toHaveBeenCalled();

    fireEvent.change(box, { target: { value: "zebra" } });
    await act(async () => void vi.advanceTimersByTime(100));
    expect(search).not.toHaveBeenCalled();
    await act(async () => void vi.advanceTimersByTime(300));
    expect(search).toHaveBeenCalledWith("zebra");

    fireEvent.click(document.querySelector("[data-action='session.open-match']")!);
    expect(screen.getByText("Rotate keys")).toBeTruthy();
    expect(onOpen).toHaveBeenCalledWith({ root: "/w/a", sessionPath: "/w/a/s1.jsonl" });
  });

  it("says so when the index cannot answer instead of showing nothing", async () => {
    setup(() => Promise.reject(new Error("down")));
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "zebra" } });
    await act(async () => void vi.advanceTimersByTime(400));
    expect(document.querySelector("[data-state='failed']")).not.toBeNull();
  });

  it("opens a grouped content match in the session's working directory", async () => {
    const onOpen = setup(() => Promise.resolve([hit]), [{ ...tree[0], sessions: [
      { ...tree[0].sessions[0], workspaceRoot: "/worktrees/a" },
    ] }]);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "zebra" } });
    await act(async () => void vi.advanceTimersByTime(400));
    fireEvent.click(document.querySelector("[data-action='session.open-match']")!);
    expect(onOpen).toHaveBeenCalledWith({ root: "/worktrees/a", sessionPath: hit.path });
  });

  it("offers to reference a match in the message box, and referencing does not open it", async () => {
    const onReference = vi.fn();
    const onOpen = setup(() => Promise.resolve([hit]), tree, onReference);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "zebra" } });
    await act(async () => void vi.advanceTimersByTime(400));
    fireEvent.click(screen.getByRole("button", { name: "引用「Rotate keys」到输入框" }));
    expect(onReference).toHaveBeenCalledExactlyOnceWith("s1", "Rotate keys");
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("has no reference button when there is no message box to put it in", async () => {
    setup(() => Promise.resolve([hit]));
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "zebra" } });
    await act(async () => void vi.advanceTimersByTime(400));
    expect(document.querySelector("[data-action='session.reference-match']")).toBeNull();
  });
});
