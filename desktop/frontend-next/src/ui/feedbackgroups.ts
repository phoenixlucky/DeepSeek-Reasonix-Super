import type { FeedbackItem, FeedbackStatus } from "../port/feedback";

export type FeedbackGroup = "all" | "fresh" | "input" | "open" | "done";

export const FEEDBACK_GROUPS: FeedbackGroup[] = ["all", "fresh", "input", "open", "done"];

const ENDED: ReadonlySet<FeedbackStatus> = new Set(["fixed", "wontfix", "duplicate", "closed", "withdrawn"]);

/** Which groups a report belongs to. `fresh` is the count of replies this visit
 *  has been told are new, not the kernel's live unread number, so a report that
 *  was just read stays where the reader found it. */
export function inGroup(item: FeedbackItem, group: FeedbackGroup, fresh: number): boolean {
  switch (group) {
    case "all": return true;
    case "fresh": return fresh > 0;
    case "input": return item.needsInput;
    case "open": return !ENDED.has(item.status);
    case "done": return ENDED.has(item.status);
  }
}
