import { useCallback, useState } from "react";

/** A past conversation the rail asked the focused pane's composer to hold as a
 *  reference. n is a counter so asking for the same one twice is two requests. */
export interface SessionRefRequest {
  n: number;
  id: string;
  title: string;
}

/** Requests keyed by pane, and the one call that makes one for the focused pane.
 *  A pane that is not open has no composer to hold it, so it gets none. */
export function usePaneSessionRefs(active: string, open: (pane: string) => boolean) {
  const [requests, setRequests] = useState<Record<string, SessionRefRequest>>({});
  const reference = useCallback((id: string, title: string) => {
    if (!open(active)) return;
    setRequests((was) => ({ ...was, [active]: { n: (was[active]?.n ?? 0) + 1, id, title } }));
  }, [active, open]);
  return [requests, reference] as const;
}
