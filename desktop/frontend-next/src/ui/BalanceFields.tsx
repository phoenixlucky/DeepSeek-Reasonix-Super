import { t } from "../i18n";
import type { ProviderEntry } from "../port/port";

const FORMAT_LABEL: Record<string, string> = {
  deepseek: "DeepSeek 格式",
  moonshot: "Moonshot 格式",
  openrouter: "OpenRouter 格式",
};

// The wallet endpoint and how it answers. The host table that implies a format
// lives in the kernel, so the entry says what its saved address implies and this
// form never guesses from a typed one.
export function BalanceFields({ entry, url, format, onUrl, onFormat }: {
  entry: ProviderEntry;
  url: string;
  format: string;
  onUrl: (v: string) => void;
  onFormat: (v: string) => void;
}) {
  const formats = entry.balanceFormats ?? [];
  const saved = url.trim() === (entry.balanceUrl ?? "").trim();
  const implied = saved ? entry.impliedBalanceFormat ?? "" : "";
  const needs = url.trim() !== "" && format === "" && saved && implied === "";
  return (
    <>
      <label className="grow full">
        <span>{t("余额地址")}</span>
        <input
          aria-label={t("余额地址")}
          data-action="provider.draft"
          data-value="balance-url"
          value={url}
          spellCheck={false}
          placeholder="https://api.deepseek.com/user/balance"
          onChange={(e) => onUrl(e.target.value)}
        />
        <i className="tip">{t("留空则不显示余额。密钥只会发给这个地址，不会跟随跳转。")}</i>
      </label>
      {url.trim() !== "" && (
        <label className="grow full">
          <span>{t("余额格式")}</span>
          <select aria-label={t("余额格式")} data-action="provider.draft" data-value="balance-format" value={format} onChange={(e) => onFormat(e.target.value)}>
            <option value="">{implied ? t("按地址推断（{format}）", { format: t(FORMAT_LABEL[implied] ?? implied) }) : t("未选择")}</option>
            {formats.map((f) => (
              <option key={f} value={f}>{t(FORMAT_LABEL[f] ?? f)}</option>
            ))}
          </select>
          <i className="tip">{t("供应商自己的域名会按它的格式读取；其他地址（例如中转站）必须选择它返回的格式，否则不读取余额。")}</i>
        </label>
      )}
      {needs && <div className="why">{t("余额格式未配置：这个地址不是供应商自己的域名，选择格式之前不会读取余额。")}</div>}
    </>
  );
}
