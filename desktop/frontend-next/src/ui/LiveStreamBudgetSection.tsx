import { useEffect, useState, useSyncExternalStore } from "react";
import { t } from "../i18n";
import { liveStreamBudget, MAX_LIVE_STREAM_BUDGET, onLiveStreamBudgetChange, setLiveStreamBudget } from "../state/prefs";
import { Group } from "./Group";

// How many open session tabs may keep a live connection at once. Empty is
// automatic. A browser holds six connections to one address and the session list
// takes one, so the window keeps a few live and reads the rest when they are shown.
export function LiveStreamBudgetSection() {
  const chosen = useSyncExternalStore(onLiveStreamBudgetChange, liveStreamBudget, liveStreamBudget);
  const [draft, setDraft] = useState(chosen === null ? "" : String(chosen));
  useEffect(() => setDraft(chosen === null ? "" : String(chosen)), [chosen]);
  const commit = () => {
    setLiveStreamBudget(draft === "" ? null : Number(draft));
    const now = liveStreamBudget();
    setDraft(now === null ? "" : String(now));
  };
  return (
    <Group
      id="live-stream-budget"
      title={t("保持实时连接的会话标签数")}
      hint={t("默认留空：自动。浏览器对同一个地址只开约 6 个连接，会话列表占一个，所以只让少数标签保持实时连接，其余的标签在显示时从断点接着读，运行中的后台标签每 5 秒读一次状态。调高会让更多后台标签实时更新，但开太多会让停止、发送排不上队。")}
    >
      <div className="lrow">
        <span className="tx">
          <label className="lb" htmlFor="live-stream-budget-input">{t("实时连接的标签数")}</label>
          <span className="ds">{t("留空表示自动；可填 1 到 {max}", { max: MAX_LIVE_STREAM_BUDGET })}</span>
        </span>
        <span className="unit-field">
          <input
            id="live-stream-budget-input"
            type="text"
            inputMode="numeric"
            placeholder={t("自动")}
            value={draft}
            onChange={(e) => setDraft(e.target.value.replace(/\D/g, ""))}
            onBlur={commit}
            data-action-change="appearance.live-stream-budget"
            data-action-keydown="appearance.live-stream-budget"
            onKeyDown={(e) => {
              if (e.key !== "Enter") return;
              commit();
              e.currentTarget.blur();
            }}
          />
          <i>{t("个")}</i>
        </span>
      </div>
    </Group>
  );
}
