import { useCallback } from "react";
import type { HubPort, TreeSession, TreeWorkspace } from "../port/hub";

/** Moving a conversation one place up or down in its project's list. The list is
 *  the kernel's, so after a move the tree is asked for again rather than patched
 *  here; while a search narrows the list there is no order to move within. */
export function useSessionMove(hub: HubPort, reload: () => Promise<void>, onError: (e: unknown) => void, searching: boolean) {
  const canMove = useCallback((ws: TreeWorkspace, session: TreeSession) => {
    const peers = ws.sessions.filter((s) => !!s.archived === !!session.archived);
    const at = peers.indexOf(session);
    return { up: !searching && at > 0, down: !searching && at >= 0 && at < peers.length - 1 };
  }, [searching]);
  const move = useCallback(async (session: TreeSession, direction: -1 | 1) => {
    try {
      await hub.moveSession(session.path, direction);
      await reload();
    } catch (e) {
      onError(e);
    }
  }, [hub, reload, onError]);
  return { canMove, move };
}
