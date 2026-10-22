import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason, savedNotApplied } from "../i18n/kernel";
import type { AgentPort } from "../port/port";
import { SaveNote, type SaveNoteState } from "./SaveNote";
import { Switch } from "./Switch";

// One switch over asking before a call that copies text an external result
// gave the task. The save sends the lists as the kernel holds them now, because
// the rules editor saves the same record and a stale copy would overwrite it.
export function ExternalOrigin({ port, onChanged }: { port: AgentPort; onChanged: () => void }) {
  const [on, setOn] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<SaveNoteState | null>(null);

  useEffect(() => {
    port.permissions().then((p) => setOn(p.askOnExternalOrigin === true)).catch(() => setOn(null));
  }, [port]);

  if (on === null) return <div className="empty">{t("无法读取这项设置。")}</div>;

  const flip = async () => {
    setBusy(true);
    setNote(null);
    try {
      const { mode, deny, ask, allow } = await port.permissions();
      const saved = await port.savePermissions({ mode, deny, ask, allow, askOnExternalOrigin: !on });
      setOn(saved.askOnExternalOrigin === true);
      onChanged();
    } catch (e) {
      setNote({ text: reason(e), unapplied: savedNotApplied(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="box">
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("询问来自外部的文字")}</span>
          <span className="ds">{t("调用逐字复制了网页或工具结果里的文字时，Auto 和 YOLO 也先问你。只认用户配置；已授权的 MCP 服务器、改写过或编码过的文字不在其内。无人可问时（无头 YOLO），这类调用会被拒绝而不是执行。修改会重建运行时，任务运行期间无法变更。默认关闭。")}</span>
        </span>
        <Switch data-action="permissions.ask-on-external-origin" on={on} busy={busy} label={t("询问来自外部的文字")} onClick={() => void flip()} />
      </div>
      <SaveNote note={note} />
    </div>
  );
}
