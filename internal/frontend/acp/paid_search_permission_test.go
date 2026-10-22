package acp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
)

func optionIDs(options []PermissionOption) []string {
	ids := make([]string, 0, len(options))
	for _, o := range options {
		ids = append(ids, o.OptionID)
	}
	return ids
}

func TestPaidSearchOffersASavedChoiceOnlyWhenTheHostSaysItCanBeSaved(t *testing.T) {
	saved := approvalOptions(control.PaidSearchApprovalTool, "deepseek_api", true, true)
	if got := strings.Join(optionIDs(saved), ","); got != "allow_once,allow_always,allow_persist,reject_once" {
		t.Fatalf("options = %s, want once, session, saved, reject", got)
	}
	for _, o := range saved {
		if o.OptionID == "allow_persist" && (o.Kind != OptAllowAlways || !strings.Contains(o.Name, "deepseek_api") || !strings.Contains(o.Name, "saved")) {
			t.Fatalf("the saved choice must say what it records and where: %+v", o)
		}
	}
	unsaved := approvalOptions(control.PaidSearchApprovalTool, "deepseek_api", true, false)
	if got := strings.Join(optionIDs(unsaved), ","); got != "allow_once,allow_always,reject_once" {
		t.Fatalf("options without a place to save = %s", got)
	}
}

func TestPaidSearchSavedChoiceReachesTheControllerAsPersist(t *testing.T) {
	fn := &fakeNotifier{onReq: func(string, any) (json.RawMessage, error) {
		return json.Marshal(PermissionRequestResult{Outcome: PermissionOutcome{Outcome: "selected", OptionID: "allow_persist"}})
	}}
	sink := newUpdateSink(fn, "sess-1")
	got := make(chan approveCall, 1)
	sink.bindApprove(func(id string, allow, session, persist bool) { got <- approveCall{id, allow, session, persist} })

	sink.Emit(event.Event{Kind: event.ApprovalRequest, Approval: event.Approval{
		ID: "21", Tool: control.PaidSearchApprovalTool, Subject: "deepseek_api", Fresh: true, AllowsSession: true, AllowsPersist: true,
	}})

	select {
	case c := <-got:
		if want := (approveCall{id: "21", allow: true, session: true, persist: true}); c != want {
			t.Fatalf("approve = %+v, want %+v", c, want)
		}
	case <-time.After(rpcCallBudget(t)):
		t.Fatal("approve was never called")
	}
}

func TestOrdinaryApprovalsKeepTheirOptionsWhateverTheHostSaysAboutSaving(t *testing.T) {
	if got := strings.Join(optionIDs(approvalOptions("bash", "git status", false, true)), ","); got != "allow_once,allow_always,reject_once" {
		t.Fatalf("an ordinary tool's options changed: %s", got)
	}
}
