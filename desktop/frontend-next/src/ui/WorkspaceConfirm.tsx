import { t } from "../i18n";

// Removing a folder closes its panes, and closing one stops what it is running.
// That price is said here rather than discovered afterwards — the kernel refuses
// the removal either way, and a refusal names no pane the reader can go find.
export function removeHint(panes: number, live: number): string {
  if (panes === 0) return t("不会删除任何文件");
  if (live === 0) return t("将先关闭 {n} 个面板；不会删除任何文件", { n: panes });
  return t("其中 {live} 个对话正在运行，停止后才能移除", { live });
}

// What deleting a conversation costs, said before it is asked.
export function sessionDeleteHint(copies: number, hasPane: boolean, live: boolean): string {
  if (copies > 0) return t("还有 {n} 份恢复副本带着这个会话没有的内容，直接删除会保留它们", { n: copies });
  return t(!hasPane ? "连同其记录一并删除" : live ? "正在运行，停止后才能删除" : "它的面板会先关掉");
}

// 确认不跟原来那行抢位置：把「×」换成「移除」两个字，宽度一变就把文件夹名挤扁
// 了。整行换成一条问句，取消永远在手边，误点的代价是零。
export function Confirm({
  what,
  hint,
  go,
  danger,
  alt,
  onGo,
  onCancel,
}: {
  what: string;
  hint?: string;
  go: string;
  danger?: boolean;
  // A wider form of the same removal, offered next to the plain one.
  alt?: { label: string; onGo: () => void };
  onGo: () => void;
  onCancel: () => void;
}) {
  return (
    <div
      className="wsconfirm"
      data-wide={alt ? "" : undefined}
      role="alertdialog"
      aria-label={what}
      data-action-keydown="layer.dismiss"
      onKeyDown={(ev) => {
        if (ev.key !== "Escape") return;
        // Dismissing the question is not stopping the run behind it.
        ev.stopPropagation();
        onCancel();
      }}
    >
      <div className="wsconfirm-t">
        <span className="q">{what}</span>
        {hint && <span className="h">{hint}</span>}
      </div>
      <div className="wsconfirm-a">
        <button data-action="layer.dismiss" onClick={onCancel}>{t("取消")}</button>
        {alt && (
          <button data-action="workspace.remove" data-value="with-copies" data-danger={danger ? "" : undefined} onClick={alt.onGo}>
            {alt.label}
          </button>
        )}
        <button autoFocus data-action="workspace.remove" data-danger={danger ? "" : undefined} onClick={onGo}>
          {go}
        </button>
      </div>
    </div>
  );
}
