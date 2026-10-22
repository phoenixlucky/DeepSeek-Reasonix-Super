import type { Subscription } from "./port";
import { mux } from "./muxclient";
import { liveStreamBudget, onLiveStreamBudgetChange } from "../state/prefs";

// A browser talking HTTP/1.1 opens at most six connections to one origin, and an
// event stream keeps its connection for as long as it lives. Every open pane held
// one, the session tree held another, and from the fifth pane on nothing else —
// Stop, submit, a status read — could get a connection: the window read as stuck.
// So only a few panes keep a live stream. The rest give theirs up and take it
// again, from the cursor they stopped at, when they have something to say.
/** The automatic budget: with the session list's stream that is four of six. */
export const LIVE_PANE_STREAMS = 3;
const POLL_MS = 5_000;

export interface Want {
  /** On screen: needs frames as they come. */
  visible: boolean;
  /** Running or waiting on the person: worth a live stream while there is room. */
  busy: boolean;
}

interface Entry {
  sub: Subscription;
  refresh: () => void;
  want: Want;
  at: number;
  live: boolean;
}

/** Decides which panes hold a live event stream. Visible panes come first, then
 *  busy ones, then the rest, the most recently changed first within each. A pane
 *  that is busy but did not make the cut is read by polling instead. */
export class StreamBudget {
  private readonly entries = new Map<string, Entry>();
  private timer: ReturnType<typeof setInterval> | null = null;
  private tick = 0;

  constructor(private readonly limit: () => number = () => LIVE_PANE_STREAMS) {}

  /** Registers a pane's subscription; the returned function removes it. */
  attach(id: string, sub: Subscription, refresh: () => void, want: Want): () => void {
    const entry: Entry = { sub, refresh, want, at: ++this.tick, live: true };
    this.entries.set(id, entry);
    this.settle();
    return () => {
      if (this.entries.get(id) === entry) this.entries.delete(id);
      this.settle();
    };
  }

  /** Says what a pane needs now. */
  update(id: string, want: Want) {
    const entry = this.entries.get(id);
    if (!entry || (entry.want.visible === want.visible && entry.want.busy === want.busy)) return;
    entry.want = want;
    entry.at = ++this.tick;
    this.settle();
  }

  /** Gives the places out again; call when the limit has changed. */
  resettle() {
    this.settle();
  }

  /** Panes with a live stream, for tests and diagnostics. */
  liveIds(): string[] {
    return [...this.entries].filter(([, e]) => e.live).map(([id]) => id);
  }

  private rank(e: Entry) {
    return e.want.visible ? 0 : e.want.busy ? 1 : 2;
  }

  private settle() {
    const order = [...this.entries].sort(([, a], [, b]) => this.rank(a) - this.rank(b) || b.at - a.at);
    let used = 0;
    order.forEach(([, entry]) => {
      const free = !entry.sub.pause || entry.sub.holdsConnection?.() === false;
      const keep = free || used < this.limit();
      if (!free && keep) used++;
      if (keep && !entry.live) entry.sub.resume?.();
      if (!keep && entry.live) entry.sub.pause?.();
      entry.live = keep;
    });
    this.poll();
  }

  private poll() {
    const owed = [...this.entries.values()].some((e) => !e.live && e.want.busy);
    if (owed && !this.timer) {
      this.timer = setInterval(() => this.entries.forEach((e) => { if (!e.live && e.want.busy) e.refresh(); }), POLL_MS);
    } else if (!owed && this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }
}

export const streamBudget = new StreamBudget(() => liveStreamBudget() ?? LIVE_PANE_STREAMS);
if (typeof window !== "undefined") {
  onLiveStreamBudgetChange(() => streamBudget.resettle());
  mux.onFallback(() => streamBudget.resettle());
}
