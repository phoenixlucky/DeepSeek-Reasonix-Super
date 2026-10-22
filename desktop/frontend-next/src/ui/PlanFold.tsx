import { useState } from "react";
import { t } from "../i18n";
import { currentStep, stepDone, stepLabel, type PlanStep } from "../state/session";
import { Plan } from "./Plan";
import type { GoalRuntime } from "../port/session";
import { goalStatusText, goalStopText, goalUsageText } from "./goalusage";
import { StudioIcon } from "./StudioIcon";

/** The list belongs above the line that narrates the turn, because that is
 *  what it is about. Mounted whether or not it is shown, so the fold survives a
 *  visit to another view. */
export function PlanFold({ plan, goal, goalStatus, goalRuntime, shown, paused }: { plan: PlanStep[]; goal?: string; goalStatus?: string; goalRuntime?: GoalRuntime; shown: boolean; paused?: boolean }) {
  const [open, setOpen] = useState(false);
  if ((plan.length === 0 && !goal) || !shown) return null;
  return (
    <div className="studio-todo" data-open={open ? "" : undefined}>
      {/* One line by default. The composer already carries the run line,
          the outbox and its own toolbar; a plan opened over all of them
          pushes the box a person types in off the bottom of a laptop. */}
      <button
        type="button"
        className="studio-todo-sum"
        data-action="plan.fold"
        aria-expanded={open}
        onClick={() => setOpen((on) => !on)}
      >
        <StudioIcon name="list" />
        <b>{t(plan.length ? "计划" : "目标")}</b>
        <span>{plan.length ? stepLabel(plan[currentStep(plan)] ?? plan[0]) : goal}</span>
        {plan.length > 0 && <small>{plan.filter(stepDone).length}/{plan.length}</small>}
        <StudioIcon name="down" className="studio-todo-fold" />
      </button>
      {open && (
        <div className="studio-todo-body">
          {goal && <p className="studio-todo-goal">{goal}</p>}
          {goal && goalRuntime && (
            <div className="studio-todo-usage" role="group" aria-label={t("目标用量")}>
              <p>{goalStatus && <b>{goalStatusText(goalStatus)}</b>}{goalStatus && " · "}{goalUsageText(goalRuntime)}</p>
              {goalRuntime.stopCause && goalStatus !== "running" && <p>{t("停下的原因：{cause}", { cause: goalStopText(goalRuntime.stopCause) })}</p>}
              {goalRuntime.lastReason && <p>{t("最近一次判断：{reason}", { reason: goalRuntime.lastReason })}</p>}
            </div>
          )}
          {plan.length > 0 && <Plan steps={plan} paused={paused} />}
        </div>
      )}
    </div>
  );
}
