import { useEffect, useState } from "react";

/** Whole seconds left until the host skips a question, ticking once a second;
 *  null when it will not be skipped. The deadline lives in the reducer, so a
 *  card remounted by scrolling counts on from where it was. */
export function useAskCountdown(skipAt: number | undefined, live: boolean): number | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!live || skipAt === undefined) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [skipAt, live]);
  if (!live || skipAt === undefined) return null;
  return Math.max(Math.ceil((skipAt - now) / 1000), 0);
}

export function clock(seconds: number): string {
  const m = Math.floor(seconds / 60);
  return m > 0 ? `${m}:${String(seconds % 60).padStart(2, "0")}` : String(seconds);
}
