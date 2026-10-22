import { describe, expect, it, vi } from "vitest";
import { fromHistory, initialState, reduce, type Item, type SessionEvent, type SessionState } from "./session";

const run = (evs: SessionEvent[]): SessionState => evs.reduce(reduce, initialState);
const users = (items: Item[]) => items.filter((i): i is Extract<Item, { t: "user" }> => i.t === "user");
const sent = (id: string): SessionEvent => ({ kind: "__user", text: "查一下", pending: false, id }) as SessionEvent;
const started = (authoredTurn: number, msgIndex: number): SessionEvent => ({ kind: "turn_started", authoredTurn, msgIndex }) as SessionEvent;

describe("when a message was sent", () => {
  it("is read off the record for a rebuilt turn, and absent for guidance that never opened one", () => {
    const [turn, steer] = users(fromHistory([
      { role: "user", content: "查一下", msgIndex: 1, createdAt: 1_760_000_000_000 },
      { role: "assistant", content: "好", msgIndex: 2 },
      { role: "user", content: "顺便", msgIndex: 3, steer: true },
    ]).items);
    expect(turn.at).toBe(1_760_000_000_000);
    expect(steer.at).toBeUndefined();
  });

  it("is the moment a live message is sent", () => {
    const before = Date.now();
    const [row] = users(run([sent("a")]).items);
    expect(row.at).toBeGreaterThanOrEqual(before);
    expect(row.at).toBeLessThanOrEqual(Date.now());
  });

  it("is when a queued follow-up's turn begins, which is the time a reload reads back", () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(1_000);
      const waiting = run([sent("a"), started(1, 1), sent("b")]);
      expect(users(waiting.items)[1].at).toBe(1_000);
      vi.setSystemTime(9_000);
      const begun = reduce(waiting, started(2, 5));
      expect(users(begun.items)[1].at).toBe(9_000);
      expect(users(begun.items)[0].at).toBe(1_000);
    } finally {
      vi.useRealTimers();
    }
  });
});
