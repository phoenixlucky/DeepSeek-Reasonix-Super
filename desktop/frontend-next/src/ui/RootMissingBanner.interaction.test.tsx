// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import "./testkit";
import { RootMissingBanner, rootMissing } from "./RootMissingBanner";
import type { Item } from "../state/session";

const tool = (id: string, refusalCode?: string, err?: string): Item => ({
  t: "tool",
  id,
  tool: { id, name: "grep", readOnly: true, ...(refusalCode || err ? { err: err ?? "root missing: x", ...(refusalCode ? { refusalCode } : {}) } : {}) },
  running: false,
  children: [],
});

afterEach(cleanup);

describe("RootMissingBanner", () => {
  it("shows when the newest tool call was refused for a missing folder, and the button asks to pick again", () => {
    const onPick = vi.fn();
    const { container } = render(<RootMissingBanner items={[tool("1"), tool("2", "tool.root_missing")]} onPick={onPick} />);
    expect(container.querySelector('[data-cause="root-missing"]')).not.toBeNull();
    fireEvent.click(container.querySelector('[data-action="root-missing.reselect"]')!);
    expect(onPick).toHaveBeenCalledTimes(1);
  });

  it("goes away once a later call ran, because the folder is back", () => {
    const { container } = render(<RootMissingBanner items={[tool("1", "tool.root_missing"), tool("2")]} onPick={() => {}} />);
    expect(container.querySelector('[data-cause="root-missing"]')).toBeNull();
  });

  it("is not shown for other refusals or an empty conversation, whatever their wording", () => {
    expect(rootMissing([])).toBe(false);
    expect(rootMissing([tool("1", "workspace.read_forbidden")])).toBe(false);
    expect(rootMissing([tool("2", undefined, "root missing: the project folder is gone")])).toBe(false);
  });
});
