import { useSyncExternalStore } from "react";
import type { AgentPort } from "../port/port";

// Whether the person is told about a level-up is the kernel's setting
// ([ui] feedback_level_notices), per kernel like the other display switches. It
// starts off, which is also the kernel's default, so an unreadable answer keeps
// the note away rather than showing it.
export interface LevelNoticePref {
  on(): boolean;
  subscribe(fn: () => void): () => void;
  adopt(on: boolean): void;
}

function createPref(): LevelNoticePref {
  let on = false;
  const listeners = new Set<() => void>();
  return {
    on: () => on,
    subscribe: (fn) => {
      listeners.add(fn);
      return () => {
        listeners.delete(fn);
      };
    },
    adopt: (next) => {
      if (on === next) return;
      on = next;
      listeners.forEach((fn) => fn());
    },
  };
}

const perPort = new WeakMap<object, LevelNoticePref>();

export function levelNoticePrefOf(port: object): LevelNoticePref {
  let pref = perPort.get(port);
  if (!pref) perPort.set(port, (pref = createPref()));
  return pref;
}

export function useShowsLevelNotice(port: object): boolean {
  const pref = levelNoticePrefOf(port);
  return useSyncExternalStore(pref.subscribe, pref.on, pref.on);
}

type PrefPort = Pick<AgentPort, "feedbackLevelNotices" | "saveFeedbackLevelNotices">;

export async function loadLevelNoticePref(port: PrefPort): Promise<boolean> {
  try {
    const got = (await port.feedbackLevelNotices()).enabled;
    levelNoticePrefOf(port).adopt(got);
    return got;
  } catch {
    return levelNoticePrefOf(port).on();
  }
}

// Shows the choice at once and asks the kernel to keep it; a refusal puts it back.
export async function saveLevelNoticePref(port: PrefPort, on: boolean): Promise<void> {
  const pref = levelNoticePrefOf(port);
  const before = pref.on();
  pref.adopt(on);
  try {
    pref.adopt((await port.saveFeedbackLevelNotices(on)).enabled);
  } catch (e) {
    pref.adopt(before);
    throw e;
  }
}
