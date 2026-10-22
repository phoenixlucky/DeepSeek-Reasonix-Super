import { useCallback, useEffect, useState } from "react";
import { reason } from "../i18n/kernel";
import type { AgentPort, PeersSettings } from "../port/port";

export interface PeersSettingsStore {
  state: PeersSettings | null;
  loadError: string;
  publish: (next: PeersSettings) => void;
}

// One read of the settings for everything that shows them: the switches and the
// allowed projects are views of one file, so a save in either one is published
// to both instead of each holding a copy that goes stale.
export function usePeersSettings(port: AgentPort): PeersSettingsStore {
  const [state, setState] = useState<PeersSettings | null>(null);
  const [loadError, setLoadError] = useState("");
  useEffect(() => {
    let alive = true;
    port.peersSettings().then((s) => alive && setState(s), (e) => alive && setLoadError(reason(e)));
    return () => { alive = false; };
  }, [port]);
  const publish = useCallback((next: PeersSettings) => setState(next), []);
  return { state, loadError, publish };
}
