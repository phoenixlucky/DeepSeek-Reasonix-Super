package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/skill"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/state/sessionstore"
)

type subagentProbe struct {
	c      *Controller
	events chan event.Event
	tasks  []string
}

func newSubagentProbe(t *testing.T, sk skill.Skill) *subagentProbe {
	t.Helper()
	p := &subagentProbe{events: make(chan event.Event, 16)}
	exec := agent.New(nil, tool.NewRegistry(), sessionstore.NewSession("parent"), agent.Options{}, event.Discard)
	p.c = New(Options{
		Executor: exec,
		Sink:     event.FuncSink(func(e event.Event) { p.events <- e }),
		Skills:   []skill.Skill{sk},
		SkillRunner: func(_ context.Context, _ skill.Skill, task string, _ skill.SubagentRunOptions) (string, error) {
			p.tasks = append(p.tasks, task)
			return "done", nil
		},
	})
	t.Cleanup(p.c.Close)
	return p
}

var reviewSkill = skill.Skill{
	Name: "review", Body: "REVIEW", RunAs: skill.RunSubagent, Scope: skill.ScopeBuiltin,
	Delivery: skill.DeliveryContract{ReviewReport: skill.ReviewReportReview},
}

var helperSkill = skill.Skill{Name: "helper", Body: "HELP", RunAs: skill.RunSubagent, Scope: skill.ScopeGlobal}

func TestAReviewSkillChosenWithNoTaskReviewsTheChangesByDefault(t *testing.T) {
	p := newSubagentProbe(t, reviewSkill)
	p.c.SubmitInvocationDisplay("", "", []InvocationRequest{{Name: "review", Kind: "subagent"}})
	waitIdle(t, p.c)
	if len(p.tasks) != 1 || strings.TrimSpace(p.tasks[0]) == "" {
		t.Fatalf("subagent tasks = %q, want one run with a default task", p.tasks)
	}
}

func TestATypedReviewSlashWithNoTaskReviewsTheChangesByDefault(t *testing.T) {
	p := newSubagentProbe(t, reviewSkill)
	p.c.Submit("/review")
	waitIdle(t, p.c)
	if len(p.tasks) != 1 || strings.TrimSpace(p.tasks[0]) == "" {
		t.Fatalf("subagent tasks = %q, want one run with a default task", p.tasks)
	}
}

func TestAReviewSkillStillTakesTheTaskItIsGiven(t *testing.T) {
	p := newSubagentProbe(t, reviewSkill)
	p.c.Submit("/review the auth changes")
	waitIdle(t, p.c)
	if len(p.tasks) != 1 || !strings.Contains(p.tasks[0], "the auth changes") {
		t.Fatalf("subagent tasks = %q", p.tasks)
	}
}

func TestASubagentSkillWithNoDefaultTaskStillNeedsOne(t *testing.T) {
	p := newSubagentProbe(t, helperSkill)
	p.c.Submit("/helper")
	p.c.SubmitInvocationDisplay("", "", []InvocationRequest{{Name: "helper", Kind: "subagent"}})
	waitIdle(t, p.c)
	if len(p.tasks) != 0 {
		t.Fatalf("a task-less call ran the subagent: %q", p.tasks)
	}
}
