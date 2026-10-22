import type { AgentPort } from "../port/port";

// Whatever the kernel answers, the list is read again afterwards: a refusal
// means the list the person was looking at no longer matches the kernel's.
export function cancelJob(port: Pick<AgentPort, "cancelJob">, id: string, notify: (e: unknown) => void, refresh: () => Promise<unknown> | void): Promise<unknown> {
  return port.cancelJob(id).catch(notify).then(refresh);
}
