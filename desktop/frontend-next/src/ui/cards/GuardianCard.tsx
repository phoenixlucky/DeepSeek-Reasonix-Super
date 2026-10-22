import type { Guardian } from "../../port/wire";
import { Sym } from "../Sym";
import { t } from "../../i18n";
import { LineRow } from "./LineRow";

export function GuardianCard({ g }: { g: Guardian }) {
  const risk = g.risk_level ?? "unknown";
  const riskLabel = risk === "low" ? t("低风险") : risk === "medium" ? t("中风险") : risk === "high" ? t("高风险") : risk === "critical" ? t("极高风险") : t("风险未知");
  // An allowed review at low or unknown risk is a step like any other: one row.
  // A refusal, or an allow that carried real risk, is something to read.
  if (g.outcome === "allow" && (risk === "low" || risk === "unknown")) {
    return <LineRow glyph="⊛" level="ok" title={`${t("守卫复核")} · ${t("已允许")}`} subject={g.subject} subjectTitle={g.subject} meta={riskLabel} detail={g.rationale || undefined} />;
  }
  return (
    <div className="call" data-k="host" data-lvl={g.outcome === "deny" ? "err" : risk === "medium" ? "warn" : undefined}>
      <div className="g">
        <Sym glyph="⊛" />
      </div>
      <div className="c">
        <div className="hl">
          {/* The tag names who is speaking; the event name belongs to the trajectory. */}
          <span className="nm">{t("守卫复核")}</span>
          <span className="tag">{t("主机")}</span>
          <span className="arg" title={g.subject}>{g.subject}</span>
        </div>
        <div className="out">
          <div className="guard" data-risk={risk}>
            <div className="guard-hd">
              <span className="verdict">{g.outcome === "deny" ? t("已拦截") : g.outcome === "allow" ? t("已允许") : g.outcome}</span>
              <span className="risk">{riskLabel}</span>
              <span className="gauge" aria-label={riskLabel}>
                <i />
                <i />
                <i />
              </span>
            </div>
            {g.rationale && <div className="guard-why">{g.rationale}</div>}
          </div>
        </div>
      </div>
    </div>
  );
}
