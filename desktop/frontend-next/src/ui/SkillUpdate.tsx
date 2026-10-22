import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, SkillUpdateChange, SkillUpdatePlan } from "../port/port";
import { HttpError } from "../port/http_error";

const STALE = "skills.update_plan_stale";
const SIGN: Record<SkillUpdateChange["kind"], string> = { added: "+", changed: "~", removed: "−" };

// The plan an update of an installed skill would carry out, read before the
// person agrees. A script is a change they have to have opened; one the plan
// could not show is read from the source folder and said so.
export function SkillUpdate({ port, name, onClose, onDone }: { port: AgentPort; name: string; onClose: () => void; onDone: () => void }) {
  const [plan, setPlan] = useState<SkillUpdatePlan | null>(null);
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [busy, setBusy] = useState(false);
  const [opened, setOpened] = useState<Set<string>>(new Set());
  const [sureTruncated, setSureTruncated] = useState(false);
  const [sureOverwrite, setSureOverwrite] = useState(false);

  const load = () => {
    setPlan(null);
    setError("");
    setStale(false);
    setOpened(new Set());
    setSureTruncated(false);
    setSureOverwrite(false);
    port.planSkillUpdate(name).then(setPlan).catch((e) => setError(reason(e)));
  };
  useEffect(load, [port, name]); // eslint-disable-line react-hooks/exhaustive-deps

  if (error && !plan) return <div className="skupd mkt addpkg"><div className="why" role="alert">{error}</div><div className="acts"><button className="act" data-action="skills.update-close" onClick={onClose}>{t("关闭")}</button></div></div>;
  if (!plan) return <div className="skupd mkt addpkg" aria-busy="true"><div className="why">{t("正在读取更新内容…")}</div></div>;

  const code = plan.changes.filter((c) => c.code);
  const text = plan.changes.filter((c) => !c.code);
  const hidden = plan.changes.filter((c) => c.truncated || c.binary);
  const unread = code.filter((c) => !c.truncated && !c.binary && !opened.has(c.path)).length;
  const record = plan.changes.length === 0 && plan.state !== "synced";
  const ready = (plan.changes.length > 0 || record) && unread === 0 && (hidden.length === 0 || sureTruncated) && (!plan.overwritesLocal || sureOverwrite);

  const apply = async () => {
    setBusy(true);
    setError("");
    try {
      await port.applySkillUpdate(plan.name, plan.planId, sureOverwrite);
      onDone();
    } catch (e) {
      setStale(e instanceof HttpError && e.reason?.code === STALE);
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  const file = (c: SkillUpdateChange) => (
    <details className="cand" key={c.path} onToggle={(e) => { if ((e.currentTarget as HTMLDetailsElement).open) setOpened((s) => new Set(s).add(c.path)); }}>
      <summary className="cand-hd">
        <span className="nm">{SIGN[c.kind]} {c.path}</span>
        <span className="meta">{t(c.kind === "added" ? "新增" : c.kind === "removed" ? "删除" : "修改")}</span>
      </summary>
      {c.truncated ? <div className="why">{t("文件太大，这里不显示内容。请到来源文件夹里查看：{path}", { path: plan.source })}</div>
        : c.binary ? <div className="why">{t("不是文本文件，这里不显示内容。")}</div>
        : <pre className="skupd-diff">{c.diff ? c.diff.replace(/\n$/, "").split("\n").map((line, i) => <span key={i} data-d={line.startsWith("---") || line.startsWith("+++") ? undefined : line[0]}>{line}{"\n"}</span>) : t("（没有可显示的差异）")}</pre>}
    </details>
  );

  return (
    <div className="skupd mkt addpkg" data-stage="confirm" aria-busy={busy}>
      <div className="find">
        <span className="t">{plan.changes.length > 0 ? t("更新 {name}：来源文件夹有 {n} 处变化", { name: plan.name, n: plan.changes.length }) : record ? t("{name} 的内容已与来源文件夹一致，只是安装记录还停在旧版本", { name: plan.name }) : t("{name} 与来源文件夹的内容一致，没有可更新的内容", { name: plan.name })}</span>
        <span className="why">{t("来源：{path}。更新后，已安装的副本会与来源文件夹完全一致。只有你点击“更新”才会执行，AI 不能触发。", { path: plan.source })}</span>
      </div>
      {plan.overwritesLocal && (
        <div className="find" data-lvl="warn">
          <span className="t">{t("已安装的副本在安装后被改过")}</span>
          <span className="why">{t("更新会让副本与来源文件夹完全一致，覆盖这些本地改动。")}</span>
          <label className="mkt-seen"><input type="checkbox" data-action="skills.update-overwrite" checked={sureOverwrite} disabled={busy} onChange={(e) => setSureOverwrite(e.target.checked)} />{t("我知道会覆盖本地改动")}</label>
        </div>
      )}
      {code.length > 0 && (
        <section className="rgrp" data-lvl="high">
          <h3>{t("以下文件不是说明文字，需要逐项展开看过")}</h3>
          {code.map(file)}
          {unread > 0 && <div className="why">{t("还有 {n} 个文件没有展开", { n: unread })}</div>}
        </section>
      )}
      {text.length > 0 && (
        <section className="rgrp" data-lvl="low">
          <h3>{t("以下文件是说明文字")}</h3>
          {text.map(file)}
        </section>
      )}
      {hidden.length > 0 && (
        <label className="mkt-seen"><input type="checkbox" data-action="skills.update-hidden" checked={sureTruncated} disabled={busy} onChange={(e) => setSureTruncated(e.target.checked)} />{t("有 {n} 个文件在这里无法显示，我已在来源文件夹里看过", { n: hidden.length })}</label>
      )}
      <div className="acts">
        <span className="note">{t("替换「我的」里已安装的副本，所有项目均可使用")}</span>
        {stale && <button className="act" data-action="skills.update-reload" disabled={busy} onClick={load}>{t("重新查看")}</button>}
        <button className="act" data-action="skills.update-close" disabled={busy} onClick={onClose}>{t("取消")}</button>
        <button className="act" data-action="skills.update-apply" data-primary disabled={busy || !ready} onClick={() => void apply()}>{t(busy ? "更新中…" : "更新")}</button>
      </div>
      {error && <div className="why" role="alert">{error}</div>}
    </div>
  );
}
