import { useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import { HttpError } from "../port/http_error";
import type { AgentPort, ObservabilityPatch, ObservabilitySettings } from "../port/port";
import { Group } from "./Group";
import { Switch } from "./Switch";

type Field = "endpoint" | "headerName" | "headerEnv" | "serviceName";

/** Hands each turn to the observability stack the person already runs, as
 *  OpenTelemetry traces. Off until they switch it on and name a collector; what
 *  is sent is structure, timing, counts and names, never prompts, tool
 *  arguments, results or paths. */
export function TraceExport({ port }: { port: AgentPort }) {
  const [state, setState] = useState<ObservabilitySettings | null>(null);
  const [draft, setDraft] = useState<Record<Field, string>>({ endpoint: "", headerName: "", headerEnv: "", serviceName: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [readOnly, setReadOnly] = useState(false);

  const adopt = useCallback((s: ObservabilitySettings) => {
    setState(s);
    setDraft({ endpoint: s.endpoint, headerName: s.headerName, headerEnv: s.headerEnv, serviceName: s.serviceName });
  }, []);

  useEffect(() => {
    let alive = true;
    port.observabilitySettings().then((s) => alive && adopt(s)).catch(() => alive && setState(null));
    return () => {
      alive = false;
    };
  }, [port, adopt]);

  const apply = useCallback(async (patch: ObservabilityPatch) => {
    setBusy(true);
    setError("");
    try {
      adopt(await port.saveObservabilitySettings(patch));
    } catch (e) {
      if (e instanceof HttpError && e.reason?.code === "observability.editing_disabled") setReadOnly(true);
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  }, [port, adopt]);

  if (!state) return null;
  const locked = busy || readOnly;
  const commit = (field: Field) => {
    if (draft[field].trim() !== state[field]) void apply({ [field]: draft[field].trim() });
  };

  return (
    <Group id="trace-export" title={t("追踪导出")} hint={t("把每轮对话以 OpenTelemetry 追踪的形式发给你自己的观测系统。默认关闭；只发送结构、耗时、数量和名称，不发送提示词、工具参数、结果或文件路径。")}>
      <div className="box">
        <div className="lrow">
          <span className="tx">
            <span className="lb">{t("导出追踪")}</span>
            <span className="ds">{!state.enabled ? t("现在是关闭的：开启并填写收集器地址后才会发送。") : state.endpoint ? t("已开启：之后新开的会话会把追踪发到下面的地址。") : t("已开启，但还没有收集器地址，所以不会发送任何东西。")}</span>
          </span>
          <Switch data-action="trace-export.enabled" on={state.enabled} busy={locked} label={t("导出追踪")} onClick={() => void apply({ enabled: !state.enabled })} />
        </div>
        <div className="wa-url">
          <label className="peernote" htmlFor="trace-endpoint">{t("收集器地址")}</label>
          <input id="trace-endpoint" className="peerfield" data-action-change="trace-export.endpoint" aria-label={t("收集器地址")} placeholder="http://localhost:4318" value={draft.endpoint} disabled={locked}
            onChange={(e) => setDraft({ ...draft, endpoint: e.target.value })} onBlur={() => commit("endpoint")} />
        </div>
        <div className="wa-url">
          <label className="peernote" htmlFor="trace-headerName">{t("认证请求头名称")}</label>
          <input id="trace-headerName" className="peerfield" data-action-change="trace-export.header-name" aria-label={t("认证请求头名称")} placeholder="Authorization" value={draft.headerName} disabled={locked}
            onChange={(e) => setDraft({ ...draft, headerName: e.target.value })} onBlur={() => commit("headerName")} />
        </div>
        <div className="wa-url">
          <label className="peernote" htmlFor="trace-headerEnv">{t("保存凭据的环境变量")}</label>
          <input id="trace-headerEnv" className="peerfield" data-action-change="trace-export.header-env" aria-label={t("保存凭据的环境变量")} placeholder="OTEL_EXPORTER_AUTH" value={draft.headerEnv} disabled={locked}
            onChange={(e) => setDraft({ ...draft, headerEnv: e.target.value })} onBlur={() => commit("headerEnv")} />
        </div>
        <div className="peernote">
          {state.headerEnv
            ? (state.headerEnvSet ? t("环境变量已设置；它的值不会显示，也不会写进配置文件。") : t("环境变量当前没有值：会不带凭据发送。"))
            : t("凭据不写在配置里：这里只填环境变量名，名称必须以 OTEL_ 或 REASONIX_OTEL_ 开头。")}
        </div>
        <div className="wa-url">
          <label className="peernote" htmlFor="trace-serviceName">{t("服务名")}</label>
          <input id="trace-serviceName" className="peerfield" data-action-change="trace-export.service-name" aria-label={t("服务名")} placeholder="reasonix" value={draft.serviceName} disabled={locked}
            onChange={(e) => setDraft({ ...draft, serviceName: e.target.value })} onBlur={() => commit("serviceName")} />
        </div>
        <div className="peernote">{t("保存后只对之后新开的会话生效，已开的会话沿用旧设置。这些设置写在用户配置文件里：")}<code>{state.path}</code></div>
        {readOnly && <div className="peernote" role="status">{t("这些设置现在是只读的：这个窗口没有被授予编辑权限。")}</div>}
        {error && <div className="why" role="alert">{error}</div>}
      </div>
    </Group>
  );
}
