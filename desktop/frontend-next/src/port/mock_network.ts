import { MockHook } from "./mock_hook";
import type { NetworkProbe, NetworkSettings } from "./port";

// Network settings and the probe that reports on them.
export class MockNetwork extends MockHook {
  private net: NetworkSettings = {
    mode: "auto",
    effective: "auto (env)",
    direct: ["api.mimo.cn"],
    endpoint: "https://api.deepseek.com/v1",
  };

  async network(): Promise<NetworkSettings> {
    return { ...this.net };
  }

  async saveNetwork(s: NetworkSettings, password: string, clearPassword: boolean) {
    this.net = {
      ...s,
      hasPassword: clearPassword ? false : s.hasPassword || !!password,
      effective:
        s.mode === "off"
          ? "不走代理（已关闭）"
          : s.mode === "custom"
            ? `${s.type || "http"}://${s.username ? s.username + ":••••@" : ""}${s.server || "?"}:${s.port || 0}`
            : "auto (env)",
    };
    return { ...this.net };
  }

  // Stands in for a real walk: a custom proxy pointing at a port nothing is
  // listening on is the common misconfiguration, so it fails at connect.

  // Stands in for a real walk: a custom proxy pointing at a port nothing is
  // listening on is the common misconfiguration, so it fails at connect.
  async diagnoseNetwork(): Promise<NetworkProbe[]> {
    const custom = this.net.mode === "custom";
    const out: NetworkProbe[] = [
      { step: "proxy", ok: true, detail: this.net.effective, durationMs: 0, code: "net.proxy.via", params: { proxy: this.net.effective } },
      { step: "dns", ok: true, detail: "proxy.corp → 10.0.0.9", durationMs: 21 },
    ];
    if (custom && (this.net.port ?? 0) === 0) {
      out.push({
        step: "connect",
        ok: false,
        detail: "连不上 proxy.corp:0：connection refused",
        durationMs: 12,
        advice: "解析得到地址但连不上代理，检查代理是不是没开、端口对不对",
        code: "net.connect.failed",
        adviceCode: "net.connect.proxy_down",
        params: { host: "proxy.corp", port: "0", error: "connection refused" },
      });
      return out;
    }
    out.push({ step: "connect", ok: true, detail: "通了", durationMs: 180, code: "net.connect.ok", params: { host: "proxy.corp", port: "8080" } });
    out.push({ step: "tls", ok: true, detail: "握手成功 · HTTP 200", durationMs: 240, code: "net.tls.ok", params: { status: 200 } });
    out.push({
      step: "auth",
      ok: false,
      detail: "key 被拒了 — HTTP 401",
      durationMs: 310,
      advice: "网络通了，是 key 的问题，去「模型」那页换一个",
      code: "net.auth.rejected",
      adviceCode: "net.auth.rejected_advice",
      params: { status: 401 },
    });
    return out;
  }
}
