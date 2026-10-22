import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { AgentPort, SkillEntry } from "../port/port";
import { Exception } from "./CapabilityScope";
import { Switch } from "./Switch";
import { SkillDeclarations } from "./SkillDeclarations";
import { SkillSpecInfo } from "./SkillSpecInfo";
import { reason } from "../i18n/kernel";
import { CopyButton } from "./CopyButton";
import { SkillUpdate } from "./SkillUpdate";
import { SkillRemove } from "./SkillRemove";

// One skill, and the two things a reader wants from it: whether the model can
// reach it at all, and which of the layers it was switched at.
const SCOPE: Record<string, string> = { global: "我的", project: "项目", custom: "自定义", builtin: "内置" };

export function SkillGroups({ skills, ...rowProps }: { skills: SkillEntry[] } & Omit<Parameters<typeof SkillRow>[0], "sk">) {
  const groups = new Map(Object.keys(SCOPE).map((scope) => [scope, [] as SkillEntry[]]));
  for (const sk of skills) {
    const scope = sk.scope ?? "";
    if (!groups.has(scope)) groups.set(scope, []);
    groups.get(scope)!.push(sk);
  }
  return <>{Array.from(groups, ([scope, rows]) => rows.length > 0 && (
    <section className="skgroup" key={scope} aria-label={t(SCOPE[scope] ?? scope) || t("其他")}>
      <h4>{t(SCOPE[scope] ?? scope) || t("其他")}</h4>
      {rows.map((sk) => <SkillRow key={sk.name} sk={sk} {...rowProps} />)}
    </section>
  ))}</>;
}

// Which way the model can reach this skill, and whether it can reach it at all.
// The second was being read back off the first by comparing the sentence — a
// judgement that a rewrite of the sentence breaks silently, because nothing
// renders differently until someone notices the mark is gone.
function triggerNote(sk: SkillEntry, implicit: boolean): { text: string; reachable: boolean } {
  if (!sk.enabled) return { text: "", reachable: true };
  const auto = !sk.manual && implicit;
  if (sk.slashName && auto) return { text: "", reachable: true };
  if (sk.slashName) return { text: "只能点名", reachable: true };
  if (auto) return { text: "只能模型自选", reachable: true };
  return { text: "调不到", reachable: false };
}

// Only the states worth interrupting a list for: in step and "cannot tell" draw nothing.
function driftNote(source: SkillEntry["source"]): { text: string; says: string } | null {
  switch (source?.state) {
    case "source_changed":
      return { text: t("源文件夹有更新"), says: t("来源文件夹里的内容比已安装的副本新：{path}", { path: source.path }) };
    case "local_edited":
      return { text: t("副本被改过"), says: t("已安装的副本与安装时不同，来源文件夹没有变化：{path}", { path: source.path }) };
    case "diverged":
      return { text: t("两边都改过"), says: t("已安装的副本和来源文件夹各自都改过：{path}", { path: source.path }) };
    case "source_missing":
      return { text: t("源文件夹不见了"), says: t("找不到来源文件夹：{path}", { path: source.path }) };
  }
  return null;
}

export function SkillRow({
  sk, implicit, port, onDone, root, onFailed,
}: {
  sk: SkillEntry; implicit: boolean; port: AgentPort; onDone: () => void; root: string;
  onFailed: (why: string) => void;
}) {
  const [owner, setOwner] = useState({ port, root });
  const currentOwner = useRef(owner);
  currentOwner.current = owner;
  const [busy, setBusy] = useState(false);
  if (owner.port !== port || owner.root !== root) {
    setOwner({ port, root });
    setBusy(false);
  }
  useEffect(() => {
    currentOwner.current = owner;
    return () => { currentOwner.current = { ...owner }; };
  }, [owner]);
  const note = triggerNote(sk, implicit);
  const drift = driftNote(sk.source);
  const [updating, setUpdating] = useState(false);
  const [removing, setRemoving] = useState(false);
  const removable = !root && !!sk.removes;
  const updatable = sk.source?.mode === "copy" && (sk.source.state === "source_changed" || sk.source.state === "local_edited" || sk.source.state === "diverged");
  const local = sk.switchScope === "project";
  const command = sk.slashName ? "/" + sk.slashName : "";
  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    onFailed("");
    try {
      await fn();
    } catch (e) {
      if (currentOwner.current === owner) onFailed(reason(e));
    } finally {
      if (currentOwner.current === owner) {
        setBusy(false);
      }
      onDone();
    }
  };
  const toggle = () => act(() => port.setSkillEnabled(sk.name, !sk.enabled, "project", root || undefined));
  return (
    <>
    <div className="skrow" data-off={sk.enabled ? undefined : ""} data-local={local ? "" : undefined}>
      <span className="nm">
        {command || sk.name}
        {command && <CopyButton key={command} text={command} iconOnly label={t("复制调用命令 {command}", { command })} />}
      </span>
      <span className="ds" title={sk.description || undefined}>{sk.description || t("未提供说明")}</span>
      <span className="how">{note.text && <i className={note.reachable ? "w" : "w none"}>{t(note.text)}</i>}</span>
      <span className="face">
        {sk.subagent && <i className="sa">{t("子代理")}</i>}
        {sk.readOnly && <i className="ro">{t("只读")}</i>}
        {drift && <i className="w" data-drift={sk.source?.state} title={drift.says} aria-label={`${drift.text}. ${drift.says}`}>{drift.text}</i>}
        {updatable && !updating && <button className="act sm" data-action="skills.update-open" data-target={sk.name} onClick={() => setUpdating(true)}>{t("从源文件夹更新…")}</button>}
        {removable && !removing && <button className="act sm" data-action="skills.remove-open" data-target={sk.name} onClick={() => setRemoving(true)}>{t("卸载…")}</button>}
      </span>
      <span className="sc" title={sk.path}>
        {sk.plugin || t(SCOPE[sk.scope ?? ""] ?? "") || sk.scope}
      </span>
      {local && <Exception onClear={() => act(() => port.clearSkillOverride(sk.name, root || undefined))} busy={busy} />}
      <Switch data-action="skill.enabled" data-target={sk.name} on={sk.enabled} busy={busy} label={t(sk.enabled ? "关闭 {name}" : "启用 {name}", { name: sk.name })} onClick={toggle} />
      <SkillDeclarations sk={sk} />
      <SkillSpecInfo sk={sk} />
    </div>
    {updating && <SkillUpdate port={port} name={sk.name} onClose={() => setUpdating(false)} onDone={() => { setUpdating(false); onDone(); }} />}
    {removing && <SkillRemove port={port} sk={sk} onClose={() => setRemoving(false)} onDone={() => { setRemoving(false); onDone(); }} />}
    </>
  );
}
