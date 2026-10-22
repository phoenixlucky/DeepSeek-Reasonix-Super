package agent

import (
	"context"
	"errors"
	"path/filepath"
	"reasonix/internal/state/sessionstore"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
)

// appendDuringSummary appends one user turn to sess while answering the summary
// request — a host append racing a fold, which is what the stale-context branch
// in foldContext exists for.
type appendDuringSummary struct {
	calls int
	sess  *sessionstore.Session
}

func (p *appendDuringSummary) Name() string { return "append-during-summary" }

func (p *appendDuringSummary) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.calls++
	if p.calls == 1 && p.sess != nil {
		p.sess.Add(provider.Message{Role: provider.RoleUser, Content: "steer: retry the failing step"})
	}
	ch := make(chan provider.Chunk, 3)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "## Files & code\n- internal/parser/lexer.go was rewritten"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func latchTranscript() []provider.Message {
	return []provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: "task"},
		{Role: provider.RoleAssistant, Content: strings.Repeat("old work ", 400)},
		{Role: provider.RoleUser, Content: "current"},
		{Role: provider.RoleAssistant, Content: "tail"},
	}
}

// A fold that loses the stale-context race must not disable automatic
// maintenance for the rest of the session: the block it records names "this
// generation", and the next turn is a new generation. One racing append
// otherwise latches compaction off in memory and on disk, and the next real
// overflow fails the turn instead of folding.
func TestStaleContextBlockDoesNotOutliveItsGeneration(t *testing.T) {
	const window = 10_000
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	sess := &sessionstore.Session{Messages: latchTranscript()}
	prov := &appendDuringSummary{sess: sess}
	a := New(prov, tool.NewRegistry(), sess, Options{
		ContextWindow: window, CompactRatio: 0.85, RecentKeep: 2,
		WorkspaceID: "workspace", ModelRef: "model",
	}, event.Discard)
	a.BindSessionPath(path, true)

	// The racing fold: the append lands mid-summary, so the commit is refused.
	if _, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure, ObservedInputTokens: 8700,
	}); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	receipt := a.sess.win.compactionState.LastReceipt
	if receipt == nil || receipt.Code != string(FailContextChanged) {
		t.Fatalf("setup: receipt = %+v, want code %q", receipt, FailContextChanged)
	}

	// A new turn's worth of closed history: the very change that made the fold
	// stale. The generation the block names is over.
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("new work ", 400)})
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "next request"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("newer work ", 400)})
	nextTurn := provider.Message{Role: provider.RoleUser, Content: "third request", CreatedAt: 42}
	sess.Add(nextTurn)
	a.activeTurnCreatedAt.Store(nextTurn.CreatedAt)
	t.Cleanup(func() { a.activeTurnCreatedAt.Store(0) })

	callsBefore := prov.calls
	if _, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure, ObservedInputTokens: 12_000,
	}); err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if prov.calls == callsBefore {
		t.Fatalf("automatic maintenance stayed blocked one turn later: summary calls = %d, projection version = %d (receipt code %q says \"blocked for this generation\")",
			prov.calls, a.window().currentProjectionVersion(), receipt.Code)
	}
}

// The same latch survives a restart: the receipt is durable, and nothing on the
// resume path releases it.
func TestPersistedStaleContextBlockKeepsAutomaticMaintenanceOff(t *testing.T) {
	const window = 10_000
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	sess := &sessionstore.Session{Messages: latchTranscript()}
	first := &appendDuringSummary{sess: sess}
	a := New(first, tool.NewRegistry(), sess, Options{
		ContextWindow: window, CompactRatio: 0.85, RecentKeep: 2,
		WorkspaceID: "workspace", ModelRef: "model",
	}, event.Discard)
	a.BindSessionPath(path, true)
	if _, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure, ObservedInputTokens: 8700,
	}); err != nil {
		t.Fatalf("first prepare: %v", err)
	}

	resumedProvider := &fakeProvider{reply: "digest after resume"}
	resumed := New(resumedProvider, tool.NewRegistry(),
		&sessionstore.Session{Messages: append([]provider.Message(nil), sess.Snapshot()...)}, Options{
			ContextWindow: window, CompactRatio: 0.85, RecentKeep: 2,
			WorkspaceID: "workspace", ModelRef: "model",
		}, event.Discard)
	resumed.BindSessionPath(path, true)
	if r := resumed.sess.win.compactionState.LastReceipt; r == nil || r.Code != string(FailContextChanged) {
		t.Fatalf("restored receipt = %+v, want the persisted %q", r, FailContextChanged)
	}

	// Plenty of closed history has accumulated; the resumed session is far above
	// the trigger and would fold today if the receipt did not stop it first.
	for range 4 {
		resumed.sess.conversation.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("grown work ", 400)})
	}
	if _, err := resumed.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerPressure, ObservedInputTokens: 13_000,
	}); err != nil {
		t.Fatalf("resumed prepare: %v", err)
	}
	if resumedProvider.got == nil {
		t.Fatalf("resumed session never asked for a summary: the persisted %q receipt latched automatic maintenance off across the restart (projection version = %d)",
			FailContextChanged, resumed.window().currentProjectionVersion())
	}
}

// And the latch is not merely a lost optimisation: at the physical ceiling it
// fails the turn, quoting a retry that never comes.
func TestStaleContextBlockFailsTheNextOverflowTurn(t *testing.T) {
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

	_, err := a.window().contextManager().Prepare(context.Background(), ContextPreparePolicy{
		Trigger: CompactionTriggerOverflow, ObservedInputTokens: 9900,
	})
	if errors.Is(err, ErrCompactionRequired) {
		t.Fatalf("an overflow one turn after a raced fold is refused instead of folded: %v (summary calls = %d)", err, prov.calls)
	}
}
