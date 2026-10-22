import { useMemo } from "react";
import { t } from "../../i18n";
import { staticDocument } from "./staticDocument";

export default function StaticHtmlPreview({ text }: { text: string }) {
  const document = useMemo(() => staticDocument(text), [text]);
  return <>
    <p className="document-notice">{t("静态预览，脚本和外部资源未加载")}</p>
    <iframe className="document-html" title={t("HTML 静态预览")} sandbox="" srcDoc={document} />
  </>;
}
