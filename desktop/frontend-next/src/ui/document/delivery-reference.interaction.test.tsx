// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { MockPort } from "../../port/mock";
import { Composer } from "../Composer";
import { DeliveryCard } from "./DeliveryCard";
import "../testkit";

afterEach(cleanup);

function DeliveryComposer({ path, submit }: { path: string; submit: (text: string) => Promise<boolean> }) {
  const [inject, setInject] = useState({ n: 0, text: "" });
  return <>
    <DeliveryCard files={[{ path }]} reply={{
      files: { file: () => vi.fn(), link: () => null, image: () => null, title: () => `C:\\Workspace\\${path}` },
      onAddFile: (reference) => setInject((was) => ({ n: was.n + 1, text: reference })),
      onQuote: vi.fn(), canRegenerate: () => false, hasLaterTurns: () => false,
      onRegenerate: vi.fn(), canBranch: () => false, onBranch: vi.fn(),
    }} />
    <Composer port={new MockPort()} status={null} running={false} inject={inject}
      onSubmit={submit} onChanged={vi.fn()} onError={vi.fn()} />
  </>;
}

it.each([
  ["docs/方案 100%.md", "@docs/方案\\ 100%.md"],
  [String.raw`before\ space.txt`, String.raw`@before\\ space.txt`],
  [String.raw`repeat\\ space.txt`, String.raw`@repeat\\\ space.txt`],
])("adds %j to the real Composer without using the Windows display title as identity", async (path, reference) => {
  const user = userEvent.setup(), submit = vi.fn().mockResolvedValue(true);
  render(<DeliveryComposer path={path} submit={submit} />);
  const box = screen.getByRole("combobox", { name: "任务输入" }) as HTMLTextAreaElement;
  await user.click(box);
  await user.keyboard("inspect ");
  await user.click(screen.getByRole("button", { name: /^更多操作/ }));
  await user.click(screen.getByRole("menuitem", { name: "添加到对话" }));
  await waitFor(() => expect(box.value).toBe(`inspect ${reference} `));
  expect(submit).not.toHaveBeenCalled();
  await user.keyboard("please{Enter}");
  await waitFor(() => expect(submit).toHaveBeenCalledWith(`inspect ${reference} please`));
});
