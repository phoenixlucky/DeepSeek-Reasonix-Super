import { useSyncExternalStore } from "react";
import { t } from "../i18n";
import { hidesLegacySessions, onHidesLegacySessionsChange, setHidesLegacySessions } from "../state/prefs";
import { Group } from "./Group";

export function LegacySessionsSection() {
  const hidden = useSyncExternalStore(onHidesLegacySessionsChange, hidesLegacySessions, hidesLegacySessions);
  return (
    <Group id="legacy-sessions" title={t("1.x 旧会话")} hint={t("从 Reasonix 1.x 迁移过来的会话是否出现在侧栏列表里。隐藏后不会删除它们，搜索时仍能找到，正在打开的旧会话也照常显示。默认显示。")}>
      <div className="seg" data-text role="group" aria-label={t("1.x 旧会话")}>
        <button data-action="appearance.legacy-sessions" data-value="show" aria-pressed={!hidden} onClick={() => setHidesLegacySessions(false)}>{t("显示")}</button>
        <button data-action="appearance.legacy-sessions" data-value="hide" aria-pressed={hidden} onClick={() => setHidesLegacySessions(true)}>{t("隐藏")}</button>
      </div>
    </Group>
  );
}
