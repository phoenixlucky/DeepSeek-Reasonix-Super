import { useState } from "react";
import type { AgentPort } from "../port/port";
import { t } from "../i18n";
import { Persona } from "./Persona";
import { StudioIcon } from "./StudioIcon";
import { usePresetLeave } from "./PresetLeave";

export function PresetPersonaPage({ port, onClose }: { port: AgentPort; onClose: () => void }) {
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const leave = usePresetLeave(dirty, busy);
  return <>
    <div className="preset-page" data-action-keydown="agent-preset.leave" onKeyDown={(e) => {
      if (e.key !== "Escape") return;
      e.preventDefault(); e.stopPropagation(); leave.request(onClose);
    }}>
      <header className="preset-page-heading">
        <button type="button" className="preset-back" data-action="agent-preset.leave" onClick={() => leave.request(onClose)}><StudioIcon name="chevron" />{t("返回预设列表")}</button>
        <h2>{t("日常搭档人设")}</h2>
        <p>{t("修改用户配置，与自定义预设独立保存。")}</p>
      </header>
      <div className="preset-page-content"><Persona port={port} onDirtyChange={setDirty} onBusyChange={setBusy} onSaved={onClose} /></div>
      <footer className="preset-page-footer"><p>{t("新会话及现有会话下一次构建时生效。")}</p>
        <button type="button" className="btn" data-action="agent-preset.leave" onClick={() => leave.request(onClose)}>{t("返回")}</button></footer>
    </div>
    {leave.confirmation}
  </>;
}
