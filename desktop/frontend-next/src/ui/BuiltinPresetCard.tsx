import { useId, useState } from "react";
import type { AgentPresetVersion } from "../port/composition";
import { t } from "../i18n";
import { StudioIcon } from "./StudioIcon";

export function BuiltinPresetCard({ version: { definition: d }, selected, onDefault, onSelect, onCopy }: {
  version: AgentPresetVersion;
  selected: boolean;
  onDefault: () => void;
  onSelect: () => void;
  onCopy: () => void;
}) {
  const [usage, setUsage] = useState(false);
  const helpId = useId();
  const name = t(d.name);
  return <article className="composition-builtin-card" data-selected={selected ? "" : undefined}>
    <button type="button" className="composition-builtin-select" data-action="agent-preset.new-task-default"
      data-target={d.id} aria-pressed={selected} aria-label={t("将{name}设为新任务默认", { name })} onClick={onDefault}>
      <span className="composition-builtin-heading"><strong>{name}</strong>
        <span className="composition-builtin-badge">{t(selected ? "新任务默认" : "内置")}</span>
        <code title={d.id}>{d.id.slice("builtin:".length)}</code></span>
      <span className="composition-builtin-description">{t(d.description)}</span>
    </button>
    <footer className="composition-builtin-footer">
      <button type="button" data-action="agent-preset.view" data-target={d.id}
        aria-label={t("查看 {name}", { name })} onClick={onSelect}>{t("模式说明")}</button>
      <button type="button" data-action="agent-preset.usage" data-target={d.id} aria-expanded={usage}
        aria-controls={helpId} aria-label={t("如何使用 {name}", { name })} onClick={() => setUsage(v => !v)}>{t("如何使用")}</button>
      <button type="button" className="composition-builtin-copy" data-action="agent-preset.copy" data-target={d.id}
        aria-label={t("复制 {name}", { name })} title={t("复制为自定义预设")} onClick={onCopy}><StudioIcon name="copy" /></button>
    </footer>
    <p className="composition-builtin-usage" id={helpId} hidden={!usage}>{t(d.id === "builtin:general"
      ? "初次使用会选择日常搭档，直接说说你想做什么。你也可以点击卡片更改新任务默认，或复制一份自定义。"
      : "在新任务输入框选择轻装搭档，再发送首条消息。需要支持读图的模型；终端操作继续遵守权限设置。")}</p>
  </article>;
}
