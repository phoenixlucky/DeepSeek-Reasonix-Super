import { useEffect, useId, useState } from "react";
import { t } from "../i18n";
import type {
  AgentComposition,
  AgentPresetCatalog,
  CompositionChoice,
  CompositionKind,
  CompositionSelection,
  CompositionModule,
} from "../port/composition";
import { COMPOSITION_KINDS } from "../port/composition";
import { Switch } from "./Switch";
import { StudioIcon } from "./StudioIcon";
import { CompositionModulePicker } from "./CompositionModulePicker";

const labels: Record<CompositionKind, string> = {
  tools: "工具",
  skills: "技能",
  mcp: "MCP 服务",
  extensions: "扩展",
  hooks: "自动化 Hook",
  commands: "斜杠命令",
};

export function CompositionEditor({
  draft,
  modules,
  disabled,
  creating = false,
  onChange,
  onValidityChange,
}: {
  draft: AgentComposition;
  modules: AgentPresetCatalog["modules"];
  disabled: boolean;
  creating?: boolean;
  onChange: (d: AgentComposition) => void;
  onValidityChange: (valid: boolean) => void;
}) {
  const [valid, setValid] = useState<Record<string, boolean>>({});
  const helpId = useId();
  useEffect(() => onValidityChange(Object.values(valid).every(Boolean)), [valid, onValidityChange]);
  const persona = draft.persona;
  const updatePersona = (patch: Partial<AgentComposition["persona"]>) =>
    onChange({ ...draft, persona: { ...persona, ...patch } });
  return (
    <div className="composition-editor">
      <label className="pers-field">
        <span className="lb">{t("预设名称")}</span>
        <input
          className="in"
          aria-label={t("预设名称")}
          data-action-change="agent-preset.name"
          disabled={disabled}
          value={draft.id.startsWith("builtin:") ? t(draft.name) : draft.name}
          onChange={(e) => onChange({ ...draft, name: e.target.value })}
        />
      </label>
      <label className="pers-field">
        <span className="lb">{t("用途说明")}</span>
        <input
          className="in"
          data-action-change="agent-preset.description"
          disabled={disabled}
          value={draft.id.startsWith("builtin:") ? t(draft.description) : draft.description}
          onChange={(e) => onChange({ ...draft, description: e.target.value })}
        />
      </label>
      <label className="pers-field">
        <span className="lb">{t("人设与要求")}</span>
        <span className="ds" id={`${helpId}-persona`}>{t("描述预设的角色、工作方式与要求；内容写在系统提示词开头。")}</span>
        <textarea
          aria-label={t("人设与要求")}
          aria-describedby={`${helpId}-persona`}
          data-action-change="persona.prefix"
          rows={5}
          maxLength={32768}
          disabled={disabled}
          value={persona.prefix}
          onChange={(e) => updatePersona({ prefix: e.target.value })}
        />
      </label>
      <label className="pers-field">
        <span className="lb">{t("补充要求")}</span>
        <span className="ds" id={`${helpId}-suffix`}>{t("放在内置系统指令之后，留空则不添加")}</span>
        <textarea
          aria-label={t("补充要求")}
          aria-describedby={`${helpId}-suffix`}
          data-action-change="persona.suffix"
          rows={5}
          maxLength={32768}
          disabled={disabled}
          value={persona.suffix}
          onChange={(e) => updatePersona({ suffix: e.target.value })}
        />
      </label>
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("不注入可选运行时上下文")}</span>
          <span className="ds">{t("开启后不注入探测到的运行环境与离线提示；指令、记忆与技能照常写入。")}</span>
        </span>
        <Switch
          data-action="persona.runtime-context"
          label={t("不注入可选运行时上下文")}
          on={!persona.include_runtime_context}
          busy={disabled}
          onClick={() => updatePersona({ include_runtime_context: !persona.include_runtime_context })}
        />
      </div>
      <div className="composition-capabilities">
        <h3>{t("能力配置")}</h3>
        <p className="ds">{t("继承当前的工具、技能与扩展配置，可按需单独调整。")}</p>
        <details>
          <summary><StudioIcon name="settings" />{t("工具")}<span>{t(draft.tools.mode === "inherit" ? "继承当前配置" : "仅使用所选项")}</span><StudioIcon name="chevron" /></summary>
          <ModuleEditor kind="tools" choices={modules?.tools ?? []} value={draft.tools} disabled={disabled}
            onValidityChange={(ok) => setValid(prev => ({ ...prev, tools: ok }))}
            onChange={(value) => onChange({ ...draft, tools: value })} />
        </details>
        <details>
          <summary><StudioIcon name="layers" />{t("技能与扩展")}<span>{t(COMPOSITION_KINDS.filter(k => k !== "tools").every(k => draft[k].mode === "inherit") ? "继承当前配置" : "自定义")}</span><StudioIcon name="chevron" /></summary>
          {COMPOSITION_KINDS.filter(kind => kind !== "tools").map((kind) => (
            <ModuleEditor key={kind} kind={kind} choices={modules?.[kind] ?? []} value={draft[kind]} disabled={disabled}
              onValidityChange={(ok) => setValid(prev => ({ ...prev, [kind]: ok }))}
              onChange={(value) => onChange({ ...draft, [kind]: value })} />
          ))}
        </details>
      </div>
      <label className="pers-field">
        <span className="lb">{t("存放范围")}</span>
        <select aria-label={t("存放范围")} aria-describedby={`${helpId}-scope`} disabled={disabled || !creating} data-action-change="agent-preset.scope"
          value={draft.id.split(":")[0]} onChange={(e) => onChange({ ...draft, id: `${e.target.value}:${draft.id.split(":")[1]}` })}>
          {draft.id.startsWith("builtin:") && <option value="builtin">{t("内置")}</option>}
          <option value="user">{t("个人")}</option><option value="project">{t("项目共享")}</option>
        </select>
        <span className="ds" id={`${helpId}-scope`}>{t("个人：保存在本机，可跨项目使用。项目共享：保存在当前项目，可随仓库共享。")}</span>
      </label>
      <label className="pers-field">
        <span className="lb">{t("预设 ID")}</span>
        <input className="in" aria-label={t("预设 ID")} aria-describedby={`${helpId}-id`} disabled={disabled || !creating} data-action-change="agent-preset.id"
          value={draft.id.split(":")[1]} onChange={(e) => onChange({ ...draft, id: `${draft.id.split(":")[0]}:${e.target.value}` })} />
        <span className="ds" id={`${helpId}-id`}>{t("自动生成，用于保存和引用预设；保存后不可修改。")}</span>
      </label>
    </div>
  );
}

function ModuleEditor({
  kind,
  choices,
  value,
  disabled,
  onChange,
  onValidityChange,
}: {
  kind: CompositionKind;
  choices: CompositionChoice[];
  value: CompositionSelection;
  disabled: boolean;
  onChange: (value: CompositionSelection) => void;
  onValidityChange: (valid: boolean) => void;
}) {
  const [jsonErrors, setJsonErrors] = useState<Record<number, string>>({});
  const [rawParameters, setRawParameters] = useState<Record<number, string>>({});
  const validate = (errors: Record<number, string>) => {
    setJsonErrors(errors);
    onValidityChange(Object.values(errors).every((v) => !v));
  };
  const changeMembers = (members: CompositionModule[]) => {
    const remap = <T,>(source: Record<number, T>): Record<number, T> =>
      Object.fromEntries(
        members.flatMap((member, index) => {
          const old = value.members.indexOf(member);
          return old in source ? [[index, source[old]]] : [];
        }),
      );
    validate(remap(jsonErrors));
    setRawParameters(remap(rawParameters));
    onChange({ ...value, members });
  };
  return (
    <fieldset className="composition-modules" disabled={disabled}>
      <legend>{t(labels[kind])}</legend>
      {kind === "hooks" && (
        <p className="ds">{t("安全 Hook 和权限规则始终生效；此处仅选择非阻断型自动化。")}</p>
      )}
      <div className="composition-module-controls">
      <select
        aria-label={`${t(labels[kind])} ${t("选择方式")}`}
        data-action-change="agent-preset.module-mode"
        data-target={kind}
        value={value.mode}
        onChange={(e) => {
          validate({});
          setRawParameters({});
          onChange({ mode: e.target.value as "inherit" | "select", members: [] });
        }}
      >
        <option value="inherit">{t("跟随当前配置，包含之后启用的资源")}</option>
        <option value="select">{t("仅使用所选项（空列表表示禁用）")}</option>
      </select>
      {value.mode === "select" && <CompositionModulePicker kind={kind} label={t(labels[kind])} choices={choices} members={value.members} disabled={disabled} onChange={changeMembers} />}
      </div>
      {value.mode === "select" && (
        <>
          {value.members.map((module, index) => (
            <div className="composition-module" key={index}>
              <div className="composition-toolbar">
                <input
                  className="in"
                  aria-label={`${t(labels[kind])} ID ${index + 1}`}
                  data-action-change="agent-preset.module-id"
                  data-target={kind}
                  value={module.id}
                  onChange={(e) =>
                    onChange({
                      ...value,
                      members: value.members.map((m, i) => (i === index ? { ...m, id: e.target.value } : m)),
                    })
                  }
                />
                <button
                  type="button"
                  className="act"
                  data-action="agent-preset.module-remove"
                  data-target={kind}
                  onClick={() => changeMembers(value.members.filter((_, i) => i !== index))}
                >
                  {t("移除")}
                </button>
              </div>
              {(kind === "mcp" || kind === "extensions") && (
                <label className="pers-field">
                  <span className="ds">{t("独立参数（JSON）；env 和 headers 仅填写 ${NAME} 引用")}</span>
                  <textarea
                    aria-label={`${t(labels[kind])} ${t("独立参数")} ${index + 1}`}
                    data-action-change="agent-preset.module-parameters"
                    data-target={kind}
                    rows={3}
                    value={rawParameters[index] ?? JSON.stringify(module.parameters ?? {}, null, 2)}
                    onChange={(e) => {
                      setRawParameters({ ...rawParameters, [index]: e.target.value });
                      try {
                        const parsed: unknown = JSON.parse(e.target.value);
                        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
                          throw new Error("object required");
                        validate({ ...jsonErrors, [index]: "" });
                        onChange({
                          ...value,
                          members: value.members.map((m, i) =>
                            i === index ? { ...m, parameters: parsed as Record<string, unknown> } : m,
                          ),
                        });
                      } catch {
                        validate({ ...jsonErrors, [index]: t("参数需要有效的 JSON 对象") });
                      }
                    }}
                  />
                  {jsonErrors[index] && <span role="alert">{jsonErrors[index]}</span>}
                </label>
              )}
            </div>
          ))}
          <details className="composition-manual">
          <summary>{t("高级：手动输入模块 ID")}</summary>
          <p className="ds">{t("仅在模块未列出时使用；填写 ID 不会安装模块或授予权限。")}</p>
          <button
            type="button"
            className="act"
            data-action="agent-preset.module-add"
            data-target={kind}
            onClick={() => onChange({ ...value, members: [...value.members, { id: "" }] })}
          >
            {t("添加空白 ID")}
          </button>
          </details>
        </>
      )}
    </fieldset>
  );
}
