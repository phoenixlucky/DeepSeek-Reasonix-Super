import { useCallback, useEffect, useRef, useState } from "react";
import { reason } from "../i18n/kernel";
import { HttpError } from "../port/http_error";
import type { AgentPort } from "../port/port";
import type { GitRepoRow } from "../port/workspace";

export interface GitRepoList {
  // Empty until answered, and empty for a workspace git knows nothing about,
  // which is not a failure.
  repos: GitRepoRow[];
  // Why the list could not be read, in words; "" when it could.
  failure: string;
  retry: () => void;
}

// A kernel that predates /git/repos has no such route: a plain 404 or 501 with no
// refusal code. That is a version, not a fault, and it keeps every older answer
// working as it did: the workspace is one repository.
const OLD_KERNEL: GitRepoRow[] = [{ dir: "", name: "", branch: "", detached: false }];
const noRoute = (e: unknown) => e instanceof HttpError && (e.status === 404 || e.status === 501) && !e.reason?.code;

// The workspace's repositories as the kernel lists them: itself when it is one,
// otherwise those found below it. Read when the pane opens and whenever `again`
// moves (the window looked back at), not after every tool call: which
// repositories exist almost never changes, and the kernel answers from what it
// kept while it has not. Requests that overlap are one request.
export function useGitRepos(port: AgentPort, again = 0): GitRepoList {
  const [state, setState] = useState<{ repos: GitRepoRow[]; failure: string }>({ repos: [], failure: "" });
  const flying = useRef(false);
  const live = useRef(true);
  useEffect(() => () => { live.current = false; }, []);
  const read = useCallback(() => {
    if (flying.current) return;
    flying.current = true;
    port.gitRepos().then(
      (r) => live.current && setState({ repos: r.repos, failure: "" }),
      (e) => live.current && setState(noRoute(e) ? { repos: OLD_KERNEL, failure: "" } : { repos: [], failure: reason(e) }),
    ).finally(() => { flying.current = false; });
  }, [port]);
  useEffect(() => { read(); }, [read, again]);
  return { ...state, retry: read };
}
