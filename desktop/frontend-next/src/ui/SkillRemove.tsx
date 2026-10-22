import { useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, SkillEntry } from "../port/port";
import { HttpError } from "../port/http_error";

const CHANGED = "skill.changed";

// What removing a skill would delete, shown before the person agrees. The
// request carries the scope and path drawn here, so a skill that changed under
// the same name since the list was read is refused, not removed.
export function SkillRemove({ port, sk, onClose, onDone }: { port: AgentPort; sk: SkillEntry; onClose: () => void; onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);

  const remove = async () => {
    setBusy(true);
    setError("");
    try {
      await port.removeSkill(sk.name, sk.scope ?? "", sk.path ?? "", undefined);
      onDone();
    } catch (e) {
      setStale(e instanceof HttpError && e.reason?.code === CHANGED);
      setError(reason(e));
      setBusy(false);
    }
  };

  return (
    <div className="skupd mkt addpkg" data-stage="confirm" aria-busy={busy}>
      <div className="find" data-lvl="warn">
        <span className="t">{t("卸载技能「{name}」？", { name: sk.name })}</span>
        <span className="why">{t(sk.removesFolder ? "会从磁盘删除下面这个文件夹及其中所有文件，不能撤销。要再用它，需要重新安装。" : "会从磁盘删除下面这个文件，不能撤销。要再用它，需要重新安装。")}</span>
        <code>{sk.removes}</code>
      </div>
      <div className="acts">
        {stale && <button className="act" data-action="skills.remove-reload" onClick={onDone}>{t("重新读取")}</button>}
        <button className="act" data-action="skills.remove-cancel" disabled={busy} onClick={onClose}>{t("取消")}</button>
        <button className="act danger" data-action="skills.remove-confirm" disabled={busy} onClick={() => void remove()}>{t(busy ? "卸载中…" : "卸载")}</button>
      </div>
      {error && <div className="why" role="alert">{error}</div>}
    </div>
  );
}
