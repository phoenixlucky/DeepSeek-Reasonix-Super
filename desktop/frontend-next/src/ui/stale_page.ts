import { useEffect, useState } from "react";
import { ownFetch } from "../port/cloud_remote";

const RECHECK_MS = 10 * 60 * 1000;

export function entryScript(html: string): string | null {
  const tag = /<script\b[^>]*\btype=["']module["'][^>]*>/i.exec(html)?.[0];
  return tag ? (/\bsrc=["']([^"']+)["']/i.exec(tag)?.[1] ?? null) : null;
}

const runningEntry = () => document.querySelector('script[type="module"][src]')?.getAttribute("src") ?? null;

// A hosted page keeps running the build it loaded, while the kernel it talks to
// moves on. Whether it is behind is read from what the host serves now: a
// different entry file is a different build. Only a clean answer says anything.
// The read goes through ownFetch rather than the global one, which a remote page
// has had replaced by a tunnel to the kernel — asking the kernel for its page
// would compare this build against a different one and always look behind.
export function useStalePage(enabled: boolean, fetchImpl: typeof fetch = ownFetch): boolean {
  const [stale, setStale] = useState(false);
  useEffect(() => {
    if (!enabled) return;
    let alive = true;
    const check = async () => {
      if (document.visibilityState === "hidden") return;
      try {
        const res = await fetchImpl("/", { cache: "no-store", credentials: "same-origin" });
        if (!res.ok) return;
        const latest = entryScript(await res.text());
        const running = runningEntry();
        if (alive && latest && running && latest !== running) setStale(true);
      } catch {
        // offline or refused: nothing is known
      }
    };
    document.addEventListener("visibilitychange", check);
    const timer = setInterval(check, RECHECK_MS);
    return () => {
      alive = false;
      document.removeEventListener("visibilitychange", check);
      clearInterval(timer);
    };
  }, [enabled]);
  return stale;
}
