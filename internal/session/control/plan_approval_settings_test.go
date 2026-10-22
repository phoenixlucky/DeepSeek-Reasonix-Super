package control

import (
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/planmode"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/state/sessionstore"
)

type planRun struct {
	c       *Controller
	cards   chan string
	notices chan string
	done    chan event.Event
}

func newPlanRun(t *testing.T) *planRun {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		textTurn("Plan:\n1. a\n2. b"),
		textTurn("done"),
	}}
	ag := agent.New(prov, tool.NewRegistry(), sessionstore.NewSession(""), agent.Options{}, event.Discard)
	r := &planRun{cards: make(chan string, 4), notices: make(chan string, 4), done: make(chan event.Event, 4)}
	r.c = New(Options{
		Runner:   ag,
		Executor: ag,
		Sink: event.FuncSink(func(e event.Event) {
			switch e.Kind {
			case event.ApprovalRequest:
				r.cards <- e.Approval.ID
			case event.Notice:
				r.notices <- e.Code
			case event.TurnDone:
				r.done <- e
			}
		}),
	})
	r.c.SetPlanMode(true)
	return r
}

// A user who chose "auto" in their own file is never shown the plan card: the
// plan starts executing and the transcript says why no one was asked.
func TestAutoPlanApprovalStartsExecutionWithoutACard(t *testing.T) {
	r := newPlanRun(t)
	if err := r.c.SavePlanApprovalSettings(true); err != nil {
		t.Fatal(err)
	}
	r.c.Send("plan something")
	select {
	case <-r.done:
	case id := <-r.cards:
		t.Fatalf("plan card %s opened under plan_approval = auto", id)
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never finished")
	}
	var noted bool
	for len(r.notices) > 0 {
		noted = noted || <-r.notices == event.NoticeCodePlanAutoApproved
	}
	if !noted {
		t.Fatal("an automatic plan start left no notice in the transcript")
	}
	if got := r.c.PlanPhase(); got == planmode.AwaitingApproval {
		t.Fatalf("phase = %v: the plan was left waiting for a click", got)
	}
}

// The default asks. Without the setting the card opens exactly as before.
func TestPlanApprovalDefaultsToTheCard(t *testing.T) {
	r := newPlanRun(t)
	if r.c.PlanApprovalSettings().Auto {
		t.Fatal("plan approval is automatic by default")
	}
	r.c.Send("plan something")
	select {
	case <-r.cards:
	case <-time.After(5 * time.Second):
		t.Fatal("the plan card never opened by default")
	}
	r.c.Cancel()
	<-r.done
}

// Choosing "ask" again takes the release back for the next plan.
func TestPlanApprovalCanBeTakenBack(t *testing.T) {
	r := newPlanRun(t)
	if err := r.c.SavePlanApprovalSettings(true); err != nil {
		t.Fatal(err)
	}
	if err := r.c.SavePlanApprovalSettings(false); err != nil {
		t.Fatal(err)
	}
	if r.c.PlanApprovalSettings().Auto || config.LoadForEdit(config.UserConfigPath()).PlanApprovalAuto() {
		t.Fatal("returning to ask did not reach the user file")
	}
}
