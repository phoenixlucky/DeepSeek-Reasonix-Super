// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { boot, STORAGE } from "../i18n";
import type { AgentPort, SkillEntry } from "../port/port";
import { SkillRow } from "./SkillRow";

beforeEach(() => {
  localStorage.setItem(STORAGE, "zh");
  boot();
});
afterEach(cleanup);

function draw(sk: SkillEntry) {
  const port = { setSkillEnabled: vi.fn(), clearSkillOverride: vi.fn() } as unknown as AgentPort;
  render(<SkillRow sk={sk} implicit port={port} root="/workspace/main" onDone={vi.fn()} onFailed={vi.fn()} />);
  return port;
}

it("shows the license, requirements and metadata a skill declares, and changes nothing", async () => {
  const port = draw({
    name: "pdf-processing", slashName: "pdf-processing", enabled: true,
    license: "Apache-2.0", compatibility: "Requires git and network access", metadata: { author: "example-org", version: "1.0" },
  });
  const summary = screen.getByText("技能信息");
  expect(summary.getAttribute("aria-label")).toBe("pdf-processing 的技能信息");
  const details = summary.closest("details")!;
  expect(details.open).toBe(false);
  await userEvent.click(summary);
  const body = within(details);
  expect(body.getByText("Apache-2.0")).toBeTruthy();
  expect(body.getByText("Requires git and network access")).toBeTruthy();
  expect(body.getByText("author")).toBeTruthy();
  expect(body.getByText("example-org")).toBeTruthy();
  expect(port.setSkillEnabled).not.toHaveBeenCalled();
});

it("draws nothing for a skill that declares none of them", () => {
  draw({ name: "plain", slashName: "plain", enabled: true });
  expect(screen.queryByText("技能信息")).toBeNull();
});
