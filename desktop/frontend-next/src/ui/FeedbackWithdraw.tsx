import { useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import { FEEDBACK_CODE, canWithdraw, type FeedbackItem } from "../port/feedback";
import { HttpError, type AgentPort } from "../port/port";

// Taking a report back closes its public issue, so the person is told that
// before anything is sent. Only the service knows whether the report already
// has an outcome; a refusal for that reason reads the list again.
export function FeedbackWithdraw({ port, item, onDone, onStale }: { port: AgentPort; item: FeedbackItem; onDone: () => void; onStale: () => void }) {
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (!canWithdraw(item)) return null;

  const withdraw = async () => {
    setBusy(true);
    setError("");
    try {
      await port.withdrawFeedback(item.receipt);
      onDone();
    } catch (e) {
      setError(reason(e));
      setBusy(false);
      if (e instanceof HttpError && e.reason?.code === FEEDBACK_CODE.notWithdrawable) onStale();
    }
  };

  if (!asking) {
    return (
      <div className="fbk-withdraw">
        <button type="button" className="btn sm" data-action="feedback.withdraw-open" data-target={item.receipt} onClick={() => setAsking(true)}>{t("撤回这条反馈…")}</button>
        {error && <p className="fbk-hint" role="alert">{error}</p>}
      </div>
    );
  }
  return (
    <div className="fbk-withdraw" data-stage="confirm" aria-busy={busy}>
      <p className="fbk-hint">
        {item.issueNumber
          ? t("撤回后维护者不再处理它，对应的 #{n} 议题会被关闭，你上传的图片会被删除；GitHub 上已经公开的文字会保留。", { n: item.issueNumber })
          : t("撤回后维护者不再处理它，你上传的图片会被删除。")}
      </p>
      <div className="fbk-withdraw-acts">
        <button type="button" className="btn sm" data-action="feedback.withdraw-cancel" data-target={item.receipt} disabled={busy} onClick={() => setAsking(false)}>{t("取消")}</button>
        <button type="button" className="btn sm danger" data-action="feedback.withdraw-confirm" data-target={item.receipt} disabled={busy} onClick={() => void withdraw()}>{t("确认撤回")}</button>
      </div>
      {error && <p className="fbk-hint" role="alert">{error}</p>}
    </div>
  );
}
