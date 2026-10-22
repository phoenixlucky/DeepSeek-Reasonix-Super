import { useEffect, useState, type ReactNode } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, DefaultModeSettings } from "../port/port";
import { Group } from "./Group";
import { Switch } from "./Switch";

// One switch over how a new conversation starts: in plan mode, or as usual. Off
// is the default. It changes only the starting mode; the mode picker still
// switches it, and a conversation that already exists keeps its own.
export function DefaultMode({ port }: { port: AgentPort }) {
  const [state, setState] = useState<DefaultModeSettings | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    port
      .defaultMode()
      .then(setState)
      .catch(() => setState(null));
  }, [port]);

  const group = (body: ReactNode) => (
    <Group id="default-mode" title={t("新对话的起始模式")} hint={t("新建对话时先进入哪种模式。只影响起点，不会改动已有的对话。")}>
      {body}
    </Group>
  );

  if (!state) return group(<div className="empty">{t("无法读取新对话的起始模式设置。")}</div>);

  const flip = async () => {
    setBusy(true);
    setError("");
    try {
      setState(await port.saveDefaultMode(!state.plan));
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  return group(
    <div className="box">
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("新对话默认进入计划模式")}</span>
          <span className="ds">{t("新建的对话先做计划，你批准后才动手。输入框里的模式按钮仍可随时切换；已有的对话保持它自己的模式。默认关闭，只写在你自己的用户配置里，项目配置不能替你改。")}</span>
        </span>
        <Switch
          data-action="default-mode.plan"
          on={state.plan}
          busy={busy}
          label={t("新对话默认进入计划模式")}
          onClick={() => void flip()}
        />
      </div>
      {error && <div className="why">{error}</div>}
    </div>,
  );
}
