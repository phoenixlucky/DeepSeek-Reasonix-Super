import { useState } from "react";
import type { Item } from "../../state/session";
import type { SurfaceSpec } from "./spec";
import { parseSurface, validSurfaceValue } from "./spec";
import { ExtensionView } from "../cards/ExtensionView";
import { SurfaceField } from "./SurfaceField";
import { t } from "../../i18n";
import { reason } from "../../i18n/kernel";
import "../RestoreNotice.css";
import type { Tool } from "../../port/wire";
import { surfaceOfTool, surfaceFields } from "./call";

export function surfaceTool(tool: Tool, running: boolean) {
  const spec = surfaceOfTool(tool);
  if (spec && surfaceFields(spec).length) return null;
  if (spec && !running && tool.output !== undefined) return <ExtensionView body={spec.body} />;
  return undefined;
}

interface Props {
  item: Extract<Item, { t: "ask" }>;
  onAnswer: (itemId: string, id: string, answers: { questionId: string; selected: string[]; value?: Record<string, unknown> }[]) => Promise<void>;
}

export function SurfaceCard(props: Props) {
  const spec = parseSurface(props.item.ask.questions[0]?.surface);
  if (!spec || props.item.surfaceExpired) return <div className="restore-notice" role="status" data-code="surface.expired">{t("界面已失效，请等待新的请求")}</div>;
  return <NativeSurface key={props.item.ask.id} {...props} spec={spec} />;
}

function NativeSurface({ item, onAnswer, spec }: Props & { spec: SurfaceSpec }) {
  const [draft, setDraft] = useState<Record<string, unknown>>(() => {
    const initial: Record<string, unknown> = Object.create(null);
    const walk = (nodes: SurfaceSpec["body"]) => nodes.forEach(n => { if (n.field?.default !== undefined) initial[n.key!] = n.field.default; if (n.children) walk(n.children); });
    walk(spec.body);
    return initial;
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const sealed = item.answered !== undefined;
  const source = sealed ? item.surfaceValues ?? {} : draft;
  const values: Record<string, unknown> = Object.create(null);
  const valueOf = (key: string) => Object.hasOwn(source, key) ? source[key] : undefined;
  let valid = true;
  const walk = (nodes: SurfaceSpec["body"]) => nodes.forEach(n => {
    if (n.field) {
      let v = valueOf(n.key!);
      if (n.field.type === "number") v = v === "" || v === undefined ? undefined : Number(v);
      if (v === undefined) { if (n.field.required) valid = false; }
      else { values[n.key!] = v; if (!validSurfaceValue(n.field, v)) valid = false; }
    }
    if (n.children) walk(n.children);
  });
  walk(spec.body);
  const send = async (dismiss = false) => {
    if (busy || sealed) return;
    setBusy(true); setError("");
    try { await onAnswer(item.id, item.ask.id, dismiss ? [] : [{ questionId: item.ask.questions[0].id, selected: [], value: values }]); }
    catch (e) { setError(reason(e)); }
    finally { setBusy(false); }
  };
  return <div className="ask" data-surface="" data-sealed={sealed ? "" : undefined} aria-busy={busy}>
    {spec.title && <div className="ask-pane" data-on=""><div className="ask-q">{spec.title}</div></div>}
    {!item.answeredElsewhere && <ExtensionView body={spec.body} renderField={node => <SurfaceField node={node} value={valueOf(node.key!)} disabled={sealed || busy} change={v => setDraft(prev => ({ ...prev, [node.key!]: v }))} />} />}
    {error && <div className="ask-hint" role="alert">{error}</div>}
    {sealed ? <div className="ask-done"><b>{item.answeredElsewhere ? t("已在其他窗口处理，请以最新运行状态为准。") : item.surfaceValues ? t("已提交") : t("未选择")}</b></div> : <div className="ask-foot">
      <button type="button" className="dismiss" data-action="ask.answer" data-value="none" disabled={busy} onClick={() => void send(true)}>{t("暂不回答")}</button>
      <button type="button" className="btn" data-primary data-action="ask.answer" data-value="chosen" disabled={!valid || busy} onClick={() => void send()}>{busy ? t("正在提交…") : t("提交")}</button>
    </div>}
  </div>;
}
