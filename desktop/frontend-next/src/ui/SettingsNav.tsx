import { Fragment } from "react";
import { t } from "../i18n";
import { ICON, NAV, type Section } from "./prefsnav";
import { arrowTabs } from "./tablist";
import { UpdateDot } from "./UpdateDot";

interface Props {
  at: Section;
  values: Record<Section, string>;
  shown: (id: Section) => boolean;
  danger: (id: Section) => boolean;
  disabled: boolean;
  onSelect: (id: Section) => void;
}

export function SettingsNav({ at, values, shown, danger, disabled, onSelect }: Props) {
  return (
    <nav className="prefs-nav" role="tablist" aria-label={t("设置分类")} onKeyDown={arrowTabs}>
      {NAV.map(([group, items]) => {
        const rows = items.filter(([id]) => shown(id));
        if (rows.length === 0) return null;
        return (
          <Fragment key={group}>
            <div className="navsec">{t(group)}</div>
            {rows.map(([id, name]) => (
              <button key={id} id={`prefs-${id}`} role="tab" title={t(name)}
                data-action="settings.section" data-value={id}
                tabIndex={at === id ? 0 : -1}
                aria-selected={at === id} disabled={disabled} onClick={() => onSelect(id)}>
                <svg viewBox="0 0 16 16" aria-hidden="true">{ICON[id]}</svg>
                {id === "versions" && <UpdateDot />}
                <span className="nm">{t(name)}</span>
                <span className="nv" title={values[id] || undefined} data-danger={danger(id) ? "" : undefined}>
                  {values[id]}
                </span>
              </button>
            ))}
          </Fragment>
        );
      })}
    </nav>
  );
}
