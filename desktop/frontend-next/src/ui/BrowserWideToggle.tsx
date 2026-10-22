import { t } from "../i18n";
import { StudioIcon } from "./StudioIcon";
import { ariaChord, chord } from "./keys";
import { TooltipButton } from "./TooltipButton";

export function BrowserWideToggle({ wide, onToggle }: { wide: boolean; onToggle: () => void }) {
  const label = wide ? t("恢复分栏") : t("加宽工作台");
  return (
    <TooltipButton className="workbench-files" data-action="browser.wide" aria-pressed={wide}
      aria-label={label} aria-keyshortcuts={`${ariaChord("B")}+Shift`}
      title={label} shortcut={chord("Shift+B")} onClick={onToggle}>
      <StudioIcon name="panel" />
    </TooltipButton>
  );
}
