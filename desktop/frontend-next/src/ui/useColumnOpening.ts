import { useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import { reason } from "../i18n/kernel";
import type { AgentPort } from "../port/port";
import { pageInFlight } from "./browser_open";

interface Opening {
  manual: boolean;
  filesAsk: unknown;
  port: AgentPort;
  agentNow: MutableRefObject<string>;
  dropBlankStart: () => void;
  follow: (key: string) => void;
  current: () => () => boolean;
  setFailed: Dispatch<SetStateAction<string>>;
}

/** What the workbench column does when it is opened. It follows the agent's page,
 *  and a page a followed link already has on its way; with neither it stays on
 *  its launcher until a browser is asked for from there. */
export function useColumnOpening({ manual, filesAsk, port, agentNow, dropBlankStart, follow, current, setFailed }: Opening) {
  const filesRequested = useRef(filesAsk);
  useEffect(() => {
    const files = filesRequested.current !== filesAsk;
    filesRequested.current = filesAsk;
    if (!manual || files) return;
    if (agentNow.current) {
      dropBlankStart();
      follow(`browser:${agentNow.current}`);
      return;
    }
    const valid = current();
    let alive = true;
    pageInFlight(port)
      ?.then((tab) => { if (alive && valid()) follow(`browser:${tab.target}`); })
      .catch((e) => { if (alive && valid()) setFailed(reason(e)); });
    return () => { alive = false; };
  }, [manual, filesAsk, agentNow, dropBlankStart, follow, port, setFailed, current]);
}
