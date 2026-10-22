import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, PlanApprovalSettings } from "../port/port";
import { Switch } from "./Switch";

// One switch over whether a submitted plan waits for the user's click. Off is
// the default. On releases only that click: tool approvals, the sandbox and
// deny rules still apply while the plan runs, and the transcript notes each
// automatic start.
export function PlanApproval({ port }: { port: AgentPort }) {
  const [state, setState] = useState<PlanApprovalSettings | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    port
      .planApproval()
      .then(setState)
      .catch(() => setState(null));
  }, [port]);

  if (!state) return <div className="empty">{t("无法读取计划批准设置。")}</div>;

  const flip = async () => {
    setBusy(true);
    setError("");
    try {
      setState(await port.savePlanApproval(!state.auto));
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="box">
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("自动批准计划")}</span>
          <span className="ds">{t("计划提交后不再等你点批准，直接开始执行，对话里会留下一条说明。只放行这一次点击：工具批准、沙箱和拒绝规则照常生效。默认关闭，只写在你自己的用户配置里，项目配置不能替你打开。")}</span>
        </span>
        <Switch
          data-action="plan-approval.auto"
          on={state.auto}
          busy={busy}
          label={t("自动批准计划")}
          onClick={() => void flip()}
        />
      </div>
      {error && <div className="why">{error}</div>}
    </div>
  );
}

// The plan-related settings share this entry point.
export { DefaultMode } from "./DefaultMode";
