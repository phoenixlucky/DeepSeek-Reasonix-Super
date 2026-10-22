import { useState } from "react";
import type { AgentPort } from "../port/port";
import type { AgentPresetVersion } from "../port/composition";
import { t } from "../i18n";

export function PresetActions({ port, version, busy, run, onDeleted, onPersona }: {
  port: AgentPort;
  version: AgentPresetVersion;
  busy: boolean;
  run: (work: () => Promise<void>) => Promise<void>;
  onDeleted: () => void;
  onPersona: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
  const [exported, setExported] = useState("");
  const [note, setNote] = useState("");
  const d = version.definition;
  return <details className="composition-extra">
    <summary>{t("更多操作")}</summary>
    {d.id === "builtin:general" && <button type="button" className="act" data-action="agent-preset.persona" disabled={busy} onClick={onPersona}>{t("编辑日常搭档人设")}</button>}
    <div className="composition-toolbar">
      <button type="button" className="act" data-action="agent-preset.export" disabled={busy} onClick={() => void run(async () => {
        setExported(JSON.stringify(await port.exportAgentPreset(d.id), null, 2));
      })}>{t("导出文件")}</button>
      {!d.id.startsWith("builtin:") && <button type="button" className="act" data-action="agent-preset.delete-preview" disabled={busy} onClick={() => setDeleting(true)}>{t("删除预设")}</button>}
    </div>
    {exported && <a className="act" download="agent-preset.json"
      href={`data:application/json;charset=utf-8,${encodeURIComponent(exported)}`}>{t("下载预设 JSON")}</a>}
    {deleting && <div className="find" role="group" aria-label={t("删除预设")}>
      <p>{t("删除后不再供新会话选择，已有会话保留绑定版本。")}</p>
      <button type="button" className="act" data-action="agent-preset.delete" disabled={busy} onClick={() => void run(async () => {
        await port.deleteAgentPreset(d.id, version.revision);
        onDeleted();
      })}>{t("确认删除")}</button>
      <button type="button" className="act" data-action="agent-preset.delete-cancel" disabled={busy} onClick={() => setDeleting(false)}>{t("取消删除")}</button>
    </div>}
    <details>
      <summary>{t("显式默认配置")}</summary>
      <p className="ds">{t("仅在显式选择 default 时使用，与本机卡片设置的新任务默认独立。")}</p>
      <div className="composition-toolbar">
        <button type="button" className="act" data-action="agent-preset.default" disabled={busy || d.id.startsWith("project:")}
          onClick={() => void run(async () => { await port.setDefaultAgentPreset(d.id, false); setNote(t("已设为个人首选预设")); })}>{t("设为个人首选")}</button>
        <button type="button" className="act" data-action="agent-preset.workspace-default" disabled={busy}
          onClick={() => void run(async () => { await port.setDefaultAgentPreset(d.id, true); setNote(t("已设为当前项目首选预设")); })}>{t("设为项目首选")}</button>
      </div>
      {note && <p role="status">{note}</p>}
    </details>
  </details>;
}
