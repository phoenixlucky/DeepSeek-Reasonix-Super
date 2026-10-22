// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { LevelUpNotice } from "./LevelUpNotice";
import { FeedbackLevelNoticeSetting } from "./FeedbackLevelNoticeSetting";
import { MockPort } from "../port/mock";
import type { AgentPort } from "../port/port";
import type { FeedbackMine, FeedbackProfile } from "../port/feedback";

afterEach(cleanup);

const profile = (level: number): FeedbackProfile => ({
  level, adoptedCount: 4, currentThreshold: 3, nextLevel: level + 1, nextThreshold: 6, remaining: 2, trustState: "active",
  trustExpiresAt: "2026-11-01T00:00:00Z", observedAt: "2026-10-07T08:00:00Z", effectiveLimits: { reportsPerHour: 6, reportsPerDay: 20, repliesPerHour: 5 },
});

function portWith(promotion: FeedbackMine["promotion"], enabled: boolean) {
  const port = new MockPort() as unknown as AgentPort;
  let pending = promotion;
  port.feedbackLevelNotices = vi.fn(async () => ({ enabled, path: "/x" }));
  port.saveFeedbackLevelNotices = vi.fn(async (on: boolean) => ({ enabled: on, path: "/x" }));
  port.myFeedback = vi.fn(async () => ({ items: [], offline: false, unread: 0, hasNew: false, profile: profile(2), promotion: pending }));
  port.ackFeedbackPromotion = vi.fn(async () => { pending = null; });
  return port;
}

const fallback = <div data-testid="fallback">other note</div>;

describe("the level-up note", () => {
  it("says the new level in words, and only a click on 知道了 acknowledges it", async () => {
    const user = userEvent.setup();
    const port = portWith({ from: 1, to: 2 }, true);
    render(<LevelUpNotice port={port} tick={0} fallback={fallback} />);
    await screen.findByText(/幼苗/);
    expect(port.ackFeedbackPromotion).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "知道了" }));
    await waitFor(() => expect(port.ackFeedbackPromotion).toHaveBeenCalledWith(2));
    await waitFor(() => expect(screen.getByTestId("fallback")).toBeTruthy());
  });

  it("is never shown with the switch off, and nothing is acknowledged by looking", async () => {
    const port = portWith({ from: 1, to: 2 }, false);
    render(<LevelUpNotice port={port} tick={0} fallback={fallback} />);
    await waitFor(() => expect(port.feedbackLevelNotices).toHaveBeenCalled());
    expect(screen.getByTestId("fallback")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "知道了" })).toBeNull();
    expect(port.ackFeedbackPromotion).not.toHaveBeenCalled();
  });

  it("reading the same list again does not show a second note after it was dismissed", async () => {
    const user = userEvent.setup();
    const port = portWith({ from: 1, to: 2 }, true);
    const { rerender } = render(<LevelUpNotice port={port} tick={0} fallback={fallback} />);
    await user.click(await screen.findByRole("button", { name: "知道了" }));
    await waitFor(() => expect(screen.getByTestId("fallback")).toBeTruthy());
    rerender(<LevelUpNotice port={port} tick={1} fallback={fallback} />);
    await waitFor(() => expect(port.myFeedback).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("button", { name: "知道了" })).toBeNull();
  });

  it("does not read the feedback list at all while the switch is off", async () => {
    const port = portWith({ from: 1, to: 2 }, false);
    render(<LevelUpNotice port={port} tick={0} fallback={fallback} />);
    await waitFor(() => expect(port.feedbackLevelNotices).toHaveBeenCalled());
    expect(port.myFeedback).not.toHaveBeenCalled();
  });
});

describe("the level notice switch", () => {
  it("is off until chosen and saves what is chosen", async () => {
    const user = userEvent.setup();
    const port = portWith(null, false);
    render(<FeedbackLevelNoticeSetting port={port} />);
    const sw = await screen.findByRole("switch", { name: "等级提升时通知我" });
    expect(sw.getAttribute("aria-checked")).toBe("false");
    await user.click(sw);
    await waitFor(() => expect(port.saveFeedbackLevelNotices).toHaveBeenCalledWith(true));
  });
});
