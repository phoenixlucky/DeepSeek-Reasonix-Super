import { useCallback, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, PeersSettings, PeersSettingsEdit } from "../port/port";
import type { PeersSettingsStore } from "./usePeersSettings";
import { Switch } from "./Switch";

/** The switches behind session messages, which used to be hand-written config.
 *  Saved values only reach the model at the next turn boundary — the tool table
 *  changes with them — so a saved edit says exactly that until the kernel does. */
export function PeerSettings({ port, settings, onSaved }: { port: AgentPort; settings: PeersSettingsStore; onSaved?: (next: PeersSettings) => void }) {
  const { state, publish } = settings;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const save = useCallback(async (edit: PeersSettingsEdit) => {
    setBusy(true);
    setError("");
    try {
      const next = await port.savePeersSettings(edit);
      publish(next);
      onSaved?.(next);
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  }, [port, publish, onSaved]);

  const shownError = error || settings.loadError;
  if (!state) return shownError ? <div className="why" role="alert">{shownError}</div> : null;
  const off = busy || !state.available;

  return (
    <div className="box">
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("启用会话间消息")}</span>
          <span className="ds">{t("同一台机器上的会话互相发文字。收到的文字先挂起，等你批准才会交给模型。")}</span>
        </span>
        <Switch data-action="peers.enable" on={state.enabled} busy={off} label={t("启用会话间消息")} onClick={() => void save({ enabled: !state.enabled })} />
      </div>
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("允许模型把任务交给别的会话")}</span>
          <span className="ds">{state.enabled ? t("给模型一个把任务交给另一个项目里会话的工具。打开就是你的总同意：派出的任务和对方的回信不再询问；新会话沿用本会话的模型和权限，不会比本会话更宽。") : t("先启用会话间消息。")}</span>
        </span>
        <Switch data-action="peers.dispatch" on={state.dispatch} busy={off || !state.enabled} label={t("允许模型把任务交给别的会话")} onClick={() => void save({ dispatch: !state.dispatch })} />
      </div>
      {!state.available && <div className="peernote" role="status">{t("此会话不支持：没有可打开的收件夹，消息无处挂起。")}</div>}
      {state.pending && <div className="peernote" role="status">{t("已保存，下个回合起生效。")}</div>}
      <div className="peernote">{t("这些开关写在用户配置文件里：")}<code>{state.path}</code></div>
      {error && <div className="why" role="alert">{error}</div>}
    </div>
  );
}
