import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { t } from "../i18n";
import type { TreeSession, TreeWorkspace } from "../port/hub";
import { useModalFocus } from "./useModalFocus";

/** Choosing the project a conversation moves to. It says what travels (the
 *  conversation and the labels the person set) and what does not (what the old
 *  project allowed: the new one decides that for itself), and that the paths
 *  recorded inside the conversation are left as they were. */
export function MoveSessionDialog({ session, label, projects, busy, error, note, onMove, onClose }: {
  session: TreeSession;
  label: string;
  projects: TreeWorkspace[];
  busy: boolean;
  error: string;
  note: string;
  onMove: (target: string) => void;
  onClose: () => void;
}) {
  const sheet = useRef<HTMLDivElement>(null);
  useModalFocus(sheet);
  const choose = useRef<HTMLSelectElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => { (choose.current ?? cancel.current)?.focus(); }, []);
  const [target, setTarget] = useState(projects[0]?.root ?? "");
  return createPortal(
    <div ref={sheet} className="pickveil" role="dialog" aria-modal="true" aria-labelledby="movess-t">
      <form className="pickcard movess" data-action="session.move-to" data-target={session.path} onSubmit={(ev) => { ev.preventDefault(); if (target) onMove(target); }}>
        <h2 id="movess-t">{t("把「{name}」移到另一个项目", { name: label })}</h2>
        <p className="picknote">{t("只带走对话本身和你设的标题、置顶、归档。原项目的权限与目标设置不会跟着走，新项目按它自己的规则重新判断；对话里记录的路径保持原样。")}</p>
        <p className="picknote">{t("检查点（回退快照）、后台任务日志和排队中的消息不随会话移动，原件会被移除。")}</p>
        {projects.length === 0 ? (
          <p className="picknote">{t("没有别的项目可以移到。先添加一个项目。")}</p>
        ) : (
          <select ref={choose} aria-label={t("目标项目")} data-action-change="session.move-to-target" value={target} disabled={busy} onChange={(ev) => setTarget(ev.target.value)}>
            {projects.map((p) => <option key={p.root} value={p.root}>{`${p.name} — ${p.root}`}</option>)}
          </select>
        )}
        {error && <p className="why" role="alert">{error}</p>}
        {note && <p className="picknote" role="status">{note}</p>}
        <div className="pickact">
          <button ref={cancel} type="button" data-action="session.move-to-cancel" disabled={busy} onClick={onClose}>{note ? t("关闭") : t("取消")}</button>
          {!note && <button type="submit" className="pickgo" data-action="session.move-to" data-target={session.path} disabled={busy || !target}>{t("移动")}</button>}
        </div>
      </form>
    </div>,
    document.body,
  );
}
