import { SseHook } from "./sse_hook";
import type { NetworkProbe, NetworkSettings, UpdateMode } from "./port";
import { SseHttp } from "./sse_http";

// How the kernel reaches the outside, and what probing that found.
export class SseNetwork extends SseHook {
  network() {
    return this.get<NetworkSettings>("/network");
  }
  saveNetwork(settings: NetworkSettings, password: string, clearPassword: boolean) {
    return this.post0<NetworkSettings>("/network", { ...settings, password, clearPassword });
  }
  async diagnoseNetwork() {
    const r = await this.post0<{ probes?: NetworkProbe[] }>("/network/diagnose");
    return r.probes ?? [];
  }
  async updateMode(): Promise<UpdateMode> {
    const res = await fetch("/studio/update-mode", { credentials: "same-origin" });
    if (!res.ok) await SseHttp.fail("/studio/update-mode", res);
    return ((await res.json()) as { mode: UpdateMode }).mode;
  }

  async setUpdateMode(mode: UpdateMode): Promise<UpdateMode> {
    const res = await fetch("/studio/update-mode", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ mode }),
    });
    if (!res.ok) await SseHttp.fail("/studio/update-mode", res);
    return ((await res.json()) as { mode: UpdateMode }).mode;
  }
}
