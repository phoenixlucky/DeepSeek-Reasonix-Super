// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { MockPort } from "../port/mock";
import { HttpError } from "../port/http_error";
import type { AgentPort } from "../port/port";
import type { GitRepoRow } from "../port/workspace";
import { WorkbenchChanges } from "./WorkbenchChanges";
import { WorkbenchGit } from "./WorkbenchGit";
import { useGitRepos } from "./useGitRepos";
import { SETTLE_MS } from "./useRepoChanges";

afterEach(() => { cleanup(); vi.useRealTimers(); });

const row = (dir: string): GitRepoRow => ({ dir, name: dir || "ws", branch: "main", detached: false });
const flush = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });

describe("the repositories are listed once, not after every tool call", () => {
  it("reads when the pane opens and again only when the window is looked back at", async () => {
    const port = new MockPort();
    const read = vi.spyOn(port, "gitRepos");
    const { rerender } = renderHook(({ again }) => useGitRepos(port as unknown as AgentPort, again), { initialProps: { again: 0 } });
    await flush();
    expect(read).toHaveBeenCalledTimes(1);
    rerender({ again: 0 });
    await flush();
    expect(read).toHaveBeenCalledTimes(1);
    rerender({ again: 1 });
    await flush();
    expect(read).toHaveBeenCalledTimes(2);
  });

  it("makes one request of requests that overlap", async () => {
    const port = new MockPort();
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    const read = vi.spyOn(port, "gitRepos").mockImplementation(async () => { await gate; return { repos: [row("")], truncated: false }; });
    const { rerender } = renderHook(({ again }) => useGitRepos(port as unknown as AgentPort, again), { initialProps: { again: 0 } });
    rerender({ again: 1 });
    rerender({ again: 2 });
    expect(read).toHaveBeenCalledTimes(1);
    release();
    await flush();
  });

  it("treats a kernel with no such route as the one repository it always was", async () => {
    const port = new MockPort();
    vi.spyOn(port, "gitRepos").mockRejectedValue(new HttpError(404, "/git/repos: 404"));
    const { result } = renderHook(() => useGitRepos(port as unknown as AgentPort));
    await flush();
    expect(result.current.failure).toBe("");
    expect(result.current.repos).toHaveLength(1);
    expect(result.current.repos[0].dir).toBe("");
    render(<WorkbenchGit port={port as unknown as AgentPort} changes={[{ path: "a.go", status: "M", unstaged: true }]} repos={result.current.repos} wrote={0} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);
    expect(screen.getByRole("button", { name: /^M\s?a\.go/ })).toBeTruthy();
  });

  it("does not take a refusal for a missing route: it says why, keeps the changes shown, and can try again", async () => {
    const port = new MockPort();
    const gitRepos = vi.spyOn(port, "gitRepos").mockRejectedValueOnce(new HttpError(500, "x", { code: "git.repos_failed" }));
    const { result } = renderHook(() => useGitRepos(port as unknown as AgentPort));
    await flush();
    expect(result.current.failure).toContain("无法列出");
    render(<WorkbenchGit port={port as unknown as AgentPort} changes={[{ path: "a.go", status: "M", unstaged: true }]} repos={result.current.repos} failure={result.current.failure} onRetry={result.current.retry} wrote={0} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);
    expect(screen.getByRole("alert").textContent).toContain("无法列出");
    expect(screen.getByRole("button", { name: /^M\s?a\.go/ })).toBeTruthy();
    gitRepos.mockResolvedValue({ repos: [row("")], truncated: false });
    await userEvent.click(screen.getByRole("button", { name: "重新读取" }));
    await flush();
    expect(result.current.failure).toBe("");
  });
});

describe("a repository is read once for a burst of writes, and only while it is open", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: false }));

  const draw = (port: MockPort, wrote: number) =>
    render(<WorkbenchGit port={port as unknown as AgentPort} changes={[]} repos={[row("api"), row("web")]} wrote={wrote} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);

  it("reads each open repository when the pane opens, then once after the writes settle", async () => {
    const port = new MockPort();
    port.repoChanges = { api: [], web: [] };
    const read = vi.spyOn(port, "changes");
    const { rerender } = draw(port, 0);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(read.mock.calls.map((c) => c[0]).sort()).toEqual(["api", "web"]);
    read.mockClear();
    for (const n of [1, 2, 3, 4, 5]) {
      rerender(<WorkbenchGit port={port as unknown as AgentPort} changes={[]} repos={[row("api"), row("web")]} wrote={n} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);
      await act(async () => { await vi.advanceTimersByTimeAsync(100); });
    }
    expect(read).not.toHaveBeenCalled();
    await act(async () => { await vi.advanceTimersByTimeAsync(SETTLE_MS); });
    expect(read.mock.calls.map((c) => c[0]).sort()).toEqual(["api", "web"]);
  });

  it("does not read a collapsed repository, and reads it when it is opened", async () => {
    const port = new MockPort();
    port.repoChanges = { api: [], web: [] };
    const read = vi.spyOn(port, "changes");
    const { rerender } = draw(port, 0);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    read.mockClear();
    await act(async () => { screen.getByRole("button", { name: /^api/ }).click(); });
    rerender(<WorkbenchGit port={port as unknown as AgentPort} changes={[]} repos={[row("api"), row("web")]} wrote={1} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(SETTLE_MS + 50); });
    expect(read.mock.calls.map((c) => c[0])).toEqual(["web"]);
    read.mockClear();
    await act(async () => { screen.getByRole("button", { name: /^api/ }).click(); });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(read.mock.calls.map((c) => c[0])).toEqual(["api"]);
  });

  it("does not run two reads of one repository at once, and reads once more when the first ends", async () => {
    const port = new MockPort();
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    const read = vi.spyOn(port, "changes").mockImplementation(async () => { await gate; return { repo: true, changes: [] }; });
    const { rerender } = render(<WorkbenchGit port={port as unknown as AgentPort} changes={[]} repos={[row("api")]} wrote={0} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(read).toHaveBeenCalledTimes(1);
    rerender(<WorkbenchGit port={port as unknown as AgentPort} changes={[]} repos={[row("api")]} wrote={1} current="" onOpen={() => {}} onChanged={() => {}} remote={false} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(SETTLE_MS + 50); });
    expect(read).toHaveBeenCalledTimes(1);
    release();
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(read).toHaveBeenCalledTimes(2);
  });
});

describe("discarding a partly staged file says both parts go", () => {
  it("adds the line only when the file is both staged and unstaged", async () => {
    const port = new MockPort() as unknown as AgentPort;
    render(<WorkbenchChanges port={port} changes={[{ path: "both.go", status: "M", staged: true, unstaged: true }, { path: "one.go", status: "M", unstaged: true }]} current="" onOpen={() => {}} onChanged={() => {}} commit={{ button: null, card: null }} />);
    await userEvent.click(screen.getByRole("button", { name: "丢弃 both.go 的改动" }));
    expect(screen.getByRole("group", { name: "确认丢弃" }).textContent).toContain("已暂存的部分和还没暂存的部分都会丢弃");
    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    await userEvent.click(screen.getByRole("button", { name: "丢弃 one.go 的改动" }));
    expect(screen.getByRole("group", { name: "确认丢弃" }).textContent).not.toContain("已暂存的部分");
  });
});
