import { t } from "../i18n";
import type { OpenWithTarget } from "../port/hub";

export function openWithName(target: OpenWithTarget): string {
  if (target.id === "file-manager") return t("文件管理器");
  if (target.id === "terminal") return t("终端");
  return target.name;
}
