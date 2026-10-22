import type { ReactNode } from "react";
import { t } from "../i18n";
import type { ProviderEntry } from "../port/port";

// How a turn's context reaches the next one. Auto is vendor detection, which is
// the only honest answer for an endpoint nobody has characterised; the other two
// are what a reader picks when the endpoint has already contradicted it.
const CONTINUATIONS: ReadonlyArray<readonly [string, string]> = [
  ["", "自动"],
  ["stateful", "引用上一轮"],
  ["stateless", "每轮完整发送"],
];

const CONTINUATION_WHY: Record<string, string> = {
  "": "按端点厂商判断。中转站若回报 previous_response_id 不受支持，改为「每轮完整发送」。",
  stateful: "只发送新的一轮，历史由端点自己保存 —— 前缀缓存命中率最高，但要求端点真的存了。",
  stateless: "每轮重发完整历史。中转站只转发、不保存状态时用这一档。",
};

export function hasEndpointSwitches(entry: ProviderEntry): boolean {
  return !!(entry.canWebSearch || entry.canSetThinking || entry.canSetContinuation);
}

// The declarations that live on the stored wire, held in the same draft as the
// rest of the form: nothing here is written until Save.
export function EndpointSwitches({
  entry, label, shape, search, sendsThinking, continuation, onSearch, onThinking, onContinuation, disabled,
}: {
  entry: ProviderEntry; label: string; shape: ReactNode; search: boolean; sendsThinking: boolean; continuation: string;
  onSearch: (on: boolean) => void; onThinking: (send: boolean) => void; onContinuation: (mode: string) => void;
  disabled: boolean;
}) {
  return (
    <div className="pdetail-options">
      {entry.canWebSearch && (
        <div className="vway">
          <span className="lb">{t("联网搜索")}</span>
          <div className="seg" role="group" aria-label={t("{name} 的联网搜索", { name: label })}>
            {[true, false].map((on) => (
              <button key={String(on)} type="button" data-action="provider.draft" data-value={`web-search:${on}`}
                aria-pressed={search === on} disabled={disabled} onClick={() => onSearch(on)}>
                {t(on ? "开" : "关")}
              </button>
            ))}
          </div>
          <span className="why">{t("端点自己执行的搜索，不占本地工具。")}</span>
        </div>
      )}
      {entry.canSetThinking && (
        <div className="vway">
          <span className="lb">{t("思考参数")}</span>
          <div className="seg" role="group" aria-label={t("{name} 的思考参数", { name: label })}>
            {[true, false].map((on) => (
              <button key={String(on)} type="button" data-action="provider.draft" data-value={`thinking:${on}`}
                aria-pressed={sendsThinking === on} disabled={disabled} onClick={() => onThinking(on)}>
                {t(on ? "自动" : "不发送")}
              </button>
            ))}
          </div>
          <span className="why">
            {t(sendsThinking ? "部分中转站不支持思考控制字段，会拒绝整个请求。遇到这种情况请切换为「不发送」。" : "只发送常规聊天参数，不再指定思考深度；模型自身的推理行为不受影响。")}
          </span>
          {sendsThinking && shape}
        </div>
      )}
      {entry.canSetContinuation && (
        <div className="vway">
          <span className="lb">{t("上下文续接")}</span>
          <div className="seg" role="group" aria-label={t("{name} 的上下文续接方式", { name: label })}>
            {CONTINUATIONS.map(([mode, text]) => (
              <button key={mode} type="button" data-action="provider.draft" data-value={`continuation:${mode || "auto"}`}
                aria-pressed={continuation === mode} disabled={disabled} onClick={() => onContinuation(mode)}>
                {t(text)}
              </button>
            ))}
          </div>
          <span className="why">{t(CONTINUATION_WHY[continuation] ?? CONTINUATION_WHY[""])}</span>
        </div>
      )}
    </div>
  );
}
