package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

// The growth release belongs to the stale-context race alone, not to every
// refusal: a fold a plan or provider hook refused is a property of the input,
// so a turn's worth of growth must not re-ask it.
func TestPlanRefusalReceiptStillHoldsAfterGrowth(t *testing.T) {
	p := &fakeProvider{reply: "digest"}
	a := newPressureAgent(t, p)
	w := a.window()
	w.recordContextMaintenanceOutcome("", CompactionTriggerPressure, "summary", "blocked", FailHookRefused, "compaction extension refused the fold")
	for _, code := range []CompactionNoopReason{FailHookRefused, FailPersistFailed, NoopCandidateAboveTrigger, NoopDigestLostEveryChange} {
		r := &sessionstore.ContextMaintenanceReceipt{Status: "blocked", Action: "summary", Code: string(code), InputTokens: 1000}
		if !w.blockedReceiptHolds(r, 1000+w.retryGrowthStep(1000)*4) {
			t.Errorf("%s: a refusal growth cannot change was released", code)
		}
	}
	growBy(a, 6000)
	_ = prepare(t, a, CompactionTriggerPressure)
	_ = prepare(t, a, CompactionTriggerPressure)
	if p.got != nil {
		t.Fatal("a plan-refused fold was retried after growth: the release is not scoped to the stale-context race")
	}
}

// A release must buy one fold, not a loop: the retry either installs or records
// a new block, and without further growth it does not re-enter the summarizer.
func TestStaleContextRetryFoldsOnceWithoutRelatching(t *testing.T) {
	const window = 10_000
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	sess := &sessionstore.Session{Messages: latchTranscript()}
	prov := &appendDuringSummary{sess: sess}
	a := New(prov, tool.NewRegistry(), sess, Options{
		ContextWindow: window, CompactRatio: 0.85, RecentKeep: 2,
		WorkspaceID: "workspace", ModelRef: "model",
	}, event.Discard)
	a.BindSessionPath(path, true)
	if _, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure, ObservedInputTokens: 8700,
	}); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	if prov.calls != 1 {
		t.Fatalf("setup: summary calls = %d, want 1", prov.calls)
	}

	// The append that made the fold stale is the growth that releases the block.
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("new work ", 400)})
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "next request"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("newer work ", 400)})
	nextTurn := provider.Message{Role: provider.RoleUser, Content: "third request", CreatedAt: 42}
	sess.Add(nextTurn)
	a.activeTurnCreatedAt.Store(nextTurn.CreatedAt)
	t.Cleanup(func() { a.activeTurnCreatedAt.Store(0) })

	if _, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure, ObservedInputTokens: 12_000,
	}); err != nil {
		t.Fatalf("released prepare: %v", err)
	}
	if prov.calls != 2 {
		t.Fatalf("released prepare made %d summary calls, want exactly one more (total 2)", prov.calls)
	}
	if r := a.sess.win.compactionState.LastReceipt; r == nil || r.Status != "applied" {
		t.Fatalf("receipt after the released fold = %+v, want an applied receipt", r)
	}

	// No further growth: the next prepare measures the installed projection
	// itself and must neither fold again nor re-record the stale-context block.
	if _, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure,
	}); err != nil {
		t.Fatalf("third prepare: %v", err)
	}
	if prov.calls != 2 {
		t.Fatalf("without further growth the retry folded again: summary calls = %d, want 2", prov.calls)
	}
	if r := a.sess.win.compactionState.LastReceipt; r != nil && r.Code == string(FailContextChanged) {
		t.Fatalf("the retry re-latched itself without growth: receipt = %+v", r)
	}
}
