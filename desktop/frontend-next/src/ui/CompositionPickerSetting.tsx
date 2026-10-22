import { useState } from "react";
import { t } from "../i18n";
import { setShowsCompositionPicker, useShowsCompositionPicker } from "../state/compositionPicker";
import { Group } from "./Group";
import { Switch } from "./Switch";

export function CompositionPickerSetting() {
  const enabled = useShowsCompositionPicker();
  const [error, setError] = useState(false);
  return <Group id="composition-picker" title={t("显示模式选择器")}
    hint={t("默认关闭。开启后在聊天输入框顶部显示模式选择器；关闭只隐藏入口，不改变会话模式。")}
    action={<Switch label={t("显示模式选择器")} data-action="agent-preset.show-picker" on={enabled}
      onClick={() => setError(!setShowsCompositionPicker(!enabled))} />}>
    {error && <p role="alert">{t("无法保存模式选择器设置，请检查本地存储权限。")}</p>}
  </Group>;
}
