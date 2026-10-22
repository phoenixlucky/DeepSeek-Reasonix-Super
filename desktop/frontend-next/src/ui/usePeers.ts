import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentPort, PeersState } from "../port/port";

const EMPTY: PeersState = { enabled: false, peers: [], held: [] };

export function usePeers(port: AgentPort | null, everyMs: number) {
  const [state, setState] = useState<PeersState>(EMPTY);
  const [failed, setFailed] = useState(false);
  const alive = useRef(true);
  const refresh = useCallback(async () => {
    if (!port) return;
    try {
      const next = await port.peers();
      if (alive.current) {
        setState(next);
        setFailed(false);
      }
    } catch {
      if (alive.current) setFailed(true);
    }
  }, [port]);
  useEffect(() => {
    alive.current = true;
    void refresh();
    const timer = everyMs > 0
      ? setInterval(() => {
          if (typeof document === "undefined" || !document.hidden) void refresh();
        }, everyMs)
      : null;
    return () => {
      alive.current = false;
      if (timer) clearInterval(timer);
    };
  }, [refresh, everyMs]);
  return { state, failed, refresh };
}
