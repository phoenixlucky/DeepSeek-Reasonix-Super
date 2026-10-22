// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import "../testkit";
import { GuardianCard } from "./GuardianCard";
import { RememberCard } from "./RememberCard";
import type { Guardian } from "../../port/wire";
import type { RememberedFact } from "../../state/session";

afterEach(cleanup);

const review = (over: Partial<Guardian>): Guardian => ({
  id: "g", tool: "bash", subject: "go test ./...", outcome: "allow", risk_level: "low", user_authorization: "high", rationale: "read-only test run", duration_ms: 1, ...over,
}) as Guardian;

// An allowed review that carried no real risk is a step like any other: one row. A
// refusal, or an allow with real risk, is something to read and keeps the card.
describe("a guardian review", () => {
  it.each([["low"], ["unknown"]])("is one row when allowed at %s risk", (risk) => {
    const box = render(<GuardianCard g={review({ risk_level: risk as Guardian["risk_level"] })} />).container;
    expect(box.querySelector(".line-row")).not.toBeNull();
    expect(box.querySelector(".call")).toBeNull();
    expect(box.querySelector(".lr-detail")?.textContent).toBe("read-only test run");
  });

  it.each([
    ["deny", "low"],
    ["allow", "medium"],
    ["allow", "high"],
    ["allow", "critical"],
  ])("stays a card for %s at %s risk", (outcome, risk) => {
    const box = render(<GuardianCard g={review({ outcome, risk_level: risk as Guardian["risk_level"] } as Partial<Guardian>)} />).container;
    expect(box.querySelector(".call")).not.toBeNull();
    expect(box.querySelector(".line-row")).toBeNull();
  });
});

const fact: RememberedFact = { name: "commit-language", title: "Commits in English", description: "All GitHub text is English", scope: "project", activation: "pinned" } as RememberedFact;

describe("a remembered fact", () => {
  it("is one row with its take-back, and the take-back names the fact", () => {
    const forget = vi.fn();
    const box = render(<RememberCard m={fact} onForget={forget} />).container;
    expect(box.querySelector(".line-row .lr-arg")?.textContent).toBe("Commits in English");
    expect(box.querySelector(".lr-detail")?.textContent).toBe("All GitHub text is English");
    const button = box.querySelector<HTMLButtonElement>('[data-action="memory.forget"]')!;
    expect(button.dataset.target).toBe("commit-language");
    fireEvent.click(button);
    expect(forget).toHaveBeenCalledWith("commit-language");
    expect(button.disabled).toBe(true);
  });

  it("loses the take-back once forgotten and reads as struck through", () => {
    const box = render(<RememberCard m={fact} forgotten onForget={() => {}} />).container;
    expect(box.querySelector('[data-action="memory.forget"]')).toBeNull();
    expect(box.querySelector(".line-row")?.hasAttribute("data-gone")).toBe(true);
  });
});
