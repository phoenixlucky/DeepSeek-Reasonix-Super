package uihub

import (
	"context"
	"testing"

	"reasonix/internal/ext/extension/protocol"
)

type recordingSession struct{ reads, submits []string }

func (r *recordingSession) ReadSession(_ context.Context, plugin string, _ protocol.SessionReadParams) (protocol.SessionReadResult, error) {
	r.reads = append(r.reads, plugin)
	return protocol.SessionReadResult{}, nil
}

func (r *recordingSession) SubmitPrompt(_ context.Context, plugin string, _ protocol.PromptSubmitParams) (protocol.PromptSubmitResult, error) {
	r.submits = append(r.submits, plugin)
	return protocol.PromptSubmitResult{Queued: true}, nil
}

type sessionCalls interface {
	ReadSession(context.Context, protocol.SessionReadParams) (protocol.SessionReadResult, error)
	SubmitPrompt(context.Context, protocol.PromptSubmitParams) (protocol.PromptSubmitResult, error)
}

// A call is attributed to the plugin its connection was bound to; an unbound or
// crashed plugin gets nothing, and a host with no session access says so.
func TestSessionCallsAreAttributedToTheBoundPluginAndNeedALivePlugin(t *testing.T) {
	rec := &recordingSession{}
	hub := New(Options{SessionID: "s", Session: rec})
	h := hub.HandlerFor("mod-x").(sessionCalls)
	if _, err := h.ReadSession(context.Background(), protocol.SessionReadParams{What: protocol.SessionReadUsage}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SubmitPrompt(context.Background(), protocol.PromptSubmitParams{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.reads) != 1 || rec.reads[0] != "mod-x" || len(rec.submits) != 1 || rec.submits[0] != "mod-x" {
		t.Fatalf("reads %v submits %v, want both credited to mod-x", rec.reads, rec.submits)
	}

	hub.ClientCrashed("mod-x")
	if _, err := h.SubmitPrompt(context.Background(), protocol.PromptSubmitParams{Text: "go"}); err == nil || len(rec.submits) != 1 {
		t.Fatalf("a crashed plugin's submit reached the host: %v, %v", err, rec.submits)
	}

	none := New(Options{SessionID: "s"}).HandlerFor("mod-y").(sessionCalls)
	if _, err := none.ReadSession(context.Background(), protocol.SessionReadParams{What: protocol.SessionReadUsage}); err == nil {
		t.Fatal("a hub with no session access served a read")
	}
}
