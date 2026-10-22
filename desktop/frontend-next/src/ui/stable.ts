import { useCallback, useRef } from "react";

/** A function whose identity never changes and which always runs the latest one
 *  given: what a memoized child needs when the parent's callback closes over
 *  state that moves every render. */
export function useStable<A extends unknown[], R>(fn: (...args: A) => R): (...args: A) => R {
  const latest = useRef(fn);
  latest.current = fn;
  return useCallback((...args: A) => latest.current(...args), []);
}

/** useStable per key: the same key always gets the same function, so a list of
 *  memoized children can each be handed a callback bound to its own id without a
 *  new closure on every render of the list. */
export function useStableFor<A extends unknown[], R>(fn: (key: string, ...args: A) => R): (key: string) => (...args: A) => R {
  const latest = useRef(fn);
  latest.current = fn;
  const cache = useRef(new Map<string, (...args: A) => R>());
  return useCallback((key: string) => {
    let bound = cache.current.get(key);
    if (!bound) {
      bound = (...args: A) => latest.current(key, ...args);
      cache.current.set(key, bound);
    }
    return bound;
  }, []);
}

/** A reload that found the same panes must hand back the objects it already had:
 *  each pane is memoized on its runtime, and a new object per reload redraws every
 *  transcript for nothing. */
export function keepSame<T extends { id: string }>(cur: T[], next: T[]): T[] {
  const reused = next.map((item) => {
    const held = cur.find((c) => c.id === item.id);
    const a = held as unknown as Record<string, unknown> | undefined;
    const b = item as unknown as Record<string, unknown>;
    return held && Object.keys({ ...a, ...b }).every((k) => a![k] === b[k]) ? held : item;
  });
  return reused.length === cur.length && reused.every((item, i) => item === cur[i]) ? cur : reused;
}
