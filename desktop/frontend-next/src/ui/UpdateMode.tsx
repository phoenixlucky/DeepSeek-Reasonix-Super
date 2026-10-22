import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, UpdateMode as Mode } from "../port/port";
import { refreshUpdateWatch } from "../state/updatewatch";

// How the window treats new releases on its own. No choice installs or restarts
// anything; the choice lives in the user's file and no project can change it.
export function UpdateMode({ port }: { port: AgentPort }) {
  const [mode, setMode] = useState<Mode | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    port.updateMode().then(setMode).catch(() => setMode(null));
  }, [port]);

  if (!mode) return null;

  const choose = async (next: Mode) => {
    if (next === mode || busy) return;
    setBusy(true);
    setError("");
    try {
      setMode(await port.setUpdateMode(next));
      void refreshUpdateWatch();
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  const modes: { id: Mode; name: string; says: string }[] = [
    { id: "manual", name: t("手动"), says: t("不会在后台联网查新版本。要更新时在上面的版本列表里自己下载并重启。这是默认。") },
    { id: "notify", name: t("提醒"), says: t("每天在后台读取一次版本目录，有更新的版本时在设置图标上显示一个小点。不会下载，也不会安装或重启。") },
    { id: "download", name: t("自动下载"), says: t("在提醒的基础上，后台下载并校验新版本。下载完成后在没有会话运行时提示你，由你点「重启并更新」；不会安装，也不会自动重启。") },
  ];
  const says = modes.find((m) => m.id === mode)?.says ?? "";
  return (
    <div className="box">
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("后台检查新版本")}</span>
          <span className="ds">{says}</span>
        </span>
        <div className="seg" data-text role="radiogroup" aria-label={t("更新方式")}>
          {modes.map((m) => (
            <button key={m.id} role="radio" data-action="update.mode" data-value={m.id} aria-checked={mode === m.id} disabled={busy} onClick={() => void choose(m.id)}>
              {m.name}
            </button>
          ))}
        </div>
      </div>
      {error && <div className="why">{error}</div>}
    </div>
  );
}
