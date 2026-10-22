// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import "./testkit";
import { useState } from "react";
import { useCompletion } from "./Completion";
import type { AgentPort } from "../port/port";
import type { Completion } from "../port/model";

afterEach(cleanup);

const answer = (text: string): Completion => ({ kind: "slash", from: 0, to: text.length, query: text, items: [{ label: text, insert: text, kind: "builtin" }] });

function drawKeystrokes(complete: (line: string, cursor: number, signal?: AbortSignal) => Promise<Completion>) {
  const port = { complete } as unknown as AgentPort;
  let type: (text: string) => void = () => {};
  function Probe() {
    const [text, setText] = useState("/");
    type = setText;
    useCompletion(port, text, text.length, () => {});
    return null;
  }
  const view = render(<Probe />);
  return { type: (text: string) => view.rerender((type(text), <Probe />)), view };
}

it("abandons the question a keystroke has made stale", async () => {
  const seen: { line: string; signal: AbortSignal }[] = [];
  const complete = vi.fn(async (line: string, _cursor: number, signal?: AbortSignal) => {
    seen.push({ line, signal: signal! });
    return answer(line);
  });
  const { type } = drawKeystrokes(complete);
  await vi.waitFor(() => expect(seen.length).toBe(1));
  type("/m");
  await vi.waitFor(() => expect(seen.length).toBe(2));
  expect(seen[0].signal.aborted).toBe(true);
  expect(seen[1].signal.aborted).toBe(false);
});

it("abandons the question when the box goes away", async () => {
  const seen: AbortSignal[] = [];
  const { view } = drawKeystrokes(async (line, _cursor, signal) => {
    seen.push(signal!);
    return answer(line);
  });
  await vi.waitFor(() => expect(seen.length).toBe(1));
  view.unmount();
  expect(seen[0].aborted).toBe(true);
});
