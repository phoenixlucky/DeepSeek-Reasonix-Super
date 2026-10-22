import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort } from "../port/port";
import { loadLevelNoticePref, saveLevelNoticePref, useShowsLevelNotice } from "../state/levelnotices";
import { Group } from "./Group";
import { Switch } from "./Switch";

export function FeedbackLevelNoticeSetting({ port }: { port: AgentPort }) {
  const on = useShowsLevelNotice(port);
  const [error, setError] = useState("");
  useEffect(() => {
    void loadLevelNoticePref(port);
  }, [port]);
  return (
    <Group id="feedback-level-notices" title={t("等级提示")} hint={t("你的反馈被采纳、等级提高时，在窗口顶部提示一次，由你点“知道了”关掉。默认关闭；关着时不会提示，也不会补报之前的升级。")}>
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("等级提升时通知我")}</span>
          <span className="ds">{t("不弹窗、不发系统通知，只在窗口顶部的提示条里说一次")}</span>
        </span>
        <Switch data-action="chrome.feedback-level-notices" on={on} label={t("等级提升时通知我")} onClick={() => {
          setError("");
          saveLevelNoticePref(port, !on).catch((e) => setError(reason(e)));
        }} />
      </div>
      {error && <div className="why">{error}</div>}
    </Group>
  );
}
