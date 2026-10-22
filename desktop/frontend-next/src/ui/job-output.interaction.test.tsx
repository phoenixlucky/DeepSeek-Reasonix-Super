// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import "./testkit";
import { DeckChips, type Deck } from "./DeckChips";
import { HttpError } from "../port/http_error";
import type { JobEntry } from "../port/port";
import type { JobOutput } from "../port/session";

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const JOBS: JobEntry[] = [{ id: "j1", kind: "bash", label: "npm run dev", status: "running", startedAt: Date.now() - 5000 }];
const out = (text: string, over: Partial<JobOutput> = {}): JobOutput => ({ text, status: "running", bytes: text.length, truncated: false, ...over });

function Host({ read, onCancelJob = async () => {} }: { read?: (id: string) => Promise<JobOutput>; onCancelJob?: (id: string) => Promise<void> }) {
  const [open, setOpen] = useState<Deck>("jobs");
  return <DeckChips tasks={[]} jobs={JOBS} open={open} onOpen={(next) => setOpen(next)} onCancelJob={onCancelJob} onReadJob={read} />;
}

const open = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(screen.getByRole("button", { name: "查看输出：npm run dev" }));

it("opens a running job's output from its row", async () => {
  const read = vi.fn().mockResolvedValue(out("ready on :3000\n"));
  render(<Host read={read} />);
  await open();
  expect(await screen.findByText(/ready on :3000/)).toBeTruthy();
  expect(read).toHaveBeenCalledWith("j1");
  expect(screen.getByRole("dialog", { name: /npm run dev/ })).toBeTruthy();
});

it("offers no output view where the window cannot read one", () => {
  render(<Host />);
  expect(screen.queryByRole("button", { name: "查看输出：npm run dev" })).toBeNull();
});

it("keeps reading while the job runs and shows what it prints next", async () => {
  const read = vi.fn().mockResolvedValueOnce(out("one\n")).mockResolvedValue(out("one\ntwo\n"));
  render(<Host read={read} />);
  await open();
  await screen.findByText(/one/);
  await act(async () => { await vi.advanceTimersByTimeAsync(2100); });
  expect(await screen.findByText(/two/)).toBeTruthy();
  expect(read.mock.calls.length).toBeGreaterThanOrEqual(2);
});

it("stops reading once the job has ended, and says it ended", async () => {
  const read = vi.fn().mockResolvedValue(out("done\n", { status: "done" }));
  render(<Host read={read} />);
  await open();
  await screen.findByText(/done/);
  const calls = read.mock.calls.length;
  await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
  expect(read.mock.calls.length).toBe(calls);
  expect(screen.getByText(/已完成/)).toBeTruthy();
});

it("says when only the end of a long output is shown", async () => {
  const read = vi.fn().mockResolvedValue(out("tail", { truncated: true, bytes: 5_000_000 }));
  render(<Host read={read} />);
  await open();
  expect(await screen.findByText(/只显示最后一部分/)).toBeTruthy();
});

it("says plainly that a job that is gone has no output left to show", async () => {
  const read = vi.fn().mockRejectedValue(new HttpError(409, "gone", { code: "job.not_running" }));
  render(<Host read={read} />);
  await open();
  expect(await screen.findByText(/这个任务已经结束/)).toBeTruthy();
});

it("closes on Escape and stops reading", async () => {
  const read = vi.fn().mockResolvedValue(out("x"));
  render(<Host read={read} />);
  await open();
  await screen.findByText("x");
  await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: /npm run dev/ })).toBeNull();
  const calls = read.mock.calls.length;
  await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
  expect(read.mock.calls.length).toBe(calls);
});

it("lets the person stop the job from the view", async () => {
  const onCancelJob = vi.fn().mockResolvedValue(undefined);
  render(<Host read={async () => out("x")} onCancelJob={onCancelJob} />);
  await open();
  await screen.findByText("x");
  await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(screen.getByRole("button", { name: "停止此任务" }));
  expect(onCancelJob).toHaveBeenCalledWith("j1");
});
