package agent

import (
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

type noticeSink struct{ events []event.Event }

func (s *noticeSink) Emit(e event.Event) { s.events = append(s.events, e) }

func fallbackNotices(sink *noticeSink) []string {
	var codes []string
	for _, e := range sink.events {
		if e.Kind == event.Notice && (e.Code == event.NoticeCodeFoldNotReusedHistory || e.Code == event.NoticeCodeFoldNotReusedLineage) {
			codes = append(codes, e.Code)
		}
	}
	return codes
}

// A saved fold that stops matching its history while the session runs sends the
// whole history from the next request on. That never happens silently: the
// request that widens the view says why, once, and keeps saying nothing after.
func TestWideningTheViewAfterAFoldStopsMatchingSaysWhyOnce(t *testing.T) {
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "s.jsonl")
	sess := sessionstore.NewSession("sys")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "task"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "answer"})
	sink := &noticeSink{}
	a := New(nil, nil, sess, Options{SessionPath: path, WorkspaceID: "ws", ModelRef: "m", ContextWindow: 1_000_000}, sink)

	// A fold that covered the first two messages, with a hash that no longer matches them.
	a.sess.win.compactionMu.Lock()
	a.sess.win.compactionState = sessionstore.CompactionState{
		SchemaVersion:  sessionstore.CompactionStateSchemaV1,
		PromptCacheKey: promptCacheKey("ws", sessionstore.BranchID(path), "m"),
		Projection: sessionstore.ContextProjection{
			Messages:          []provider.Message{{Role: provider.RoleSystem, Content: "digest"}},
			CoveredCount:      2,
			CoveredPrefixHash: "stale",
		},
	}
	a.sess.win.compactionMu.Unlock()

	first := a.window().visibleByFullScan()
	if len(first) != len(sess.Snapshot()) {
		t.Fatalf("the invalid fold should widen to the whole history, got %d messages", len(first))
	}
	if got := fallbackNotices(sink); len(got) != 1 || got[0] != event.NoticeCodeFoldNotReusedHistory {
		t.Fatalf("widening notices = %v, want one %q", got, event.NoticeCodeFoldNotReusedHistory)
	}
	a.window().visibleByFullScan()
	if got := fallbackNotices(sink); len(got) != 1 {
		t.Fatalf("a second request repeated the notice: %v", got)
	}
}
