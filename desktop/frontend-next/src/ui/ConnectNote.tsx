import { t } from "../i18n";

export const UNCONFIGURED_REF = "unconfigured/none";

export interface ConnectOffer {
  none: boolean;
  open: () => void;
}

export function ConnectNote({ offer }: { offer: ConnectOffer }) {
  return (
    <div className="rtbar posturenote" data-kind="connect" data-lvl="warn" role="status">
      <span className="t">{offer.none ? t("还没连接模型服务") : t("当前模型还没有 API key")}</span>
      <button data-action="onboarding.reopen" onClick={offer.open}>{t("去连接")}</button>
    </div>
  );
}
