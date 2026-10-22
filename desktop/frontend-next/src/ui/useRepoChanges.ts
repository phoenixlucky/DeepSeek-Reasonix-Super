import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentPort, WorkspaceChange } from "../port/port";

// How long writes are let to pile up before a repository is read: one agent turn
// is dozens of tool calls, and the repository is read once for all of them.
export const SETTLE_MS = 700;

// The changes of one repository below the workspace. A repository that is
// collapsed is not read at all, and is read the moment it is opened; the first
// read and a read the person asked for (reload) are immediate; reads that the
// agent's writes ask for wait until the writes settle. A read asked for while one
// is running is made once more when that one ends, never alongside it.
export function useRepoChanges(port: AgentPort, repo: string, open: boolean, wrote: number) {
  const [changes, setChanges] = useState<WorkspaceChange[]>([]);
  const [asked, setAsked] = useState(0);
  const flying = useRef(false);
  const again = useRef(false);
  const live = useRef(true);
  const immediate = useRef(true);
  useEffect(() => () => { live.current = false; }, []);

  const read = useCallback(() => {
    if (flying.current) {
      again.current = true;
      return;
    }
    flying.current = true;
    port.changes(repo).then(
      (r) => live.current && setChanges(r.changes),
      () => live.current && setChanges([]),
    ).finally(() => {
      flying.current = false;
      if (again.current && live.current) {
        again.current = false;
        read();
      }
    });
  }, [port, repo]);

  useEffect(() => { if (open) immediate.current = true; }, [open]);
  useEffect(() => {
    if (!open) return;
    const wait = immediate.current ? 0 : SETTLE_MS;
    immediate.current = false;
    const timer = setTimeout(read, wait);
    return () => clearTimeout(timer);
  }, [open, wrote, asked, read]);

  const reload = useCallback(() => {
    immediate.current = true;
    setAsked((n) => n + 1);
  }, []);
  return { changes, reload };
}
