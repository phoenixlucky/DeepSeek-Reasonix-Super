package i18n

// UnattendedMessages is the text of the notices a run gives when the host
// goes on without a person: a retry, a timeout, an automatic approval. They
// sit in their own group, embedded by NotifyMessages, so each new one adds a
// line here and not to the catalogue files that are at their size ceiling.
type UnattendedMessages struct {
	GoalRetryFmt         string // a Goal paused on its completion check is being retried, one %d attempt
	GoalRetryGaveUpFmt   string // the retry allowance ran out, one %d count
	AwaitTimedOutFmt     string // nobody answered a handed-back list; one %s need
	AwaitGaveUpFmt       string // the continuations ran out; one %d count
	EgressAutoAllowedFmt string // bash reached a host outside allowed_domains without asking; one %s host
	PlanAutoApproved     string // the plan card answered itself
}

var unattendedEN = UnattendedMessages{
	GoalRetryFmt:         "retrying the Goal after its completion check failed (attempt %d)",
	GoalRetryGaveUpFmt:   "the completion check failed %d times in a row, so the Goal waits for you now",
	AwaitTimedOutFmt:     "nobody answered \"%s\", so the task list continues on its own judgement",
	AwaitGaveUpFmt:       "nobody answered %d times in a row, so the task list waits for you now",
	EgressAutoAllowedFmt: "bash reached %s without asking (sandbox.egress_unlisted = \"allow\" in your user config)",
	PlanAutoApproved:     "plan approved automatically (agent.plan_approval = \"auto\" in your user config)",
}

var unattendedZH = UnattendedMessages{
	GoalRetryFmt:         "目标的完成度检查失败，自动重试（第 %d 次）",
	GoalRetryGaveUpFmt:   "完成度检查连续失败 %d 次，目标不再自动重试，等你处理",
	AwaitTimedOutFmt:     "没有人回应“%s”，任务清单按模型自己的判断继续",
	AwaitGaveUpFmt:       "连续 %d 次没有人回应，任务清单不再自动继续，等你输入",
	EgressAutoAllowedFmt: "bash 访问了 %s，未询问（用户配置 sandbox.egress_unlisted = \"allow\"）",
	PlanAutoApproved:     "计划已自动批准（用户配置 agent.plan_approval = \"auto\"）",
}

var unattendedZHTW = UnattendedMessages{
	GoalRetryFmt:         "目標的完成度檢查失敗，自動重試（第 %d 次）",
	GoalRetryGaveUpFmt:   "完成度檢查連續失敗 %d 次，目標不再自動重試，等你處理",
	AwaitTimedOutFmt:     "沒有人回應「%s」，任務清單依模型自己的判斷繼續",
	AwaitGaveUpFmt:       "連續 %d 次沒有人回應，任務清單不再自動繼續，等你輸入",
	EgressAutoAllowedFmt: "bash 存取了 %s，未詢問（使用者設定 sandbox.egress_unlisted = \"allow\"）",
	PlanAutoApproved:     "計畫已自動批准（使用者設定 agent.plan_approval = \"auto\"）",
}
