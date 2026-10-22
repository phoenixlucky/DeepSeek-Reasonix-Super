package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent/testutil"
	"reasonix/internal/state/sessionstore"
)

// overflowingProvider refuses a summary request larger than limitChars the way a
// provider refuses an input past its context: a client error with the documented code.
type overflowingProvider struct {
	mu         sync.Mutex
	limitChars int
	sizes      []int
}

func (p *overflowingProvider) Name() string { return "overflowing" }

func (p *overflowingProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	n := 0
	for _, m := range req.Messages {
		n += len(m.Content)
	}
	p.mu.Lock()
	p.sizes = append(p.sizes, n)
	p.mu.Unlock()
	if n > p.limitChars {
		return nil, &provider.APIError{Provider: "overflowing", Status: 400, Body: `{"error":{"code":"context_length_exceeded","message":"input is too long"}}`}
	}
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: testutil.SummaryReply(req, "kept the facts")}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, FinishReason: "stop"}}
	close(ch)
	return ch, nil
}

// A fold of ordinary assistant text, large enough that the summary input budget is
// in play: a window of 200K tokens and about 90K tokens of history.
func largeFoldableSession() *sessionstore.Session {
	big := strings.Repeat("word ", 2000)
	msgs := []provider.Message{{Role: provider.RoleSystem, Content: "sys"}, {Role: provider.RoleUser, Content: "standing constraint: keep the API"}}
	for range 36 {
		msgs = append(msgs, provider.Message{Role: provider.RoleAssistant, Content: big}, provider.Message{Role: provider.RoleUser, Content: "continue"})
	}
	return &sessionstore.Session{Messages: msgs}
}

// A summary request the provider refuses as too large is not the end of the fold:
// it is retried smaller, and the digest lands.
func TestARejectedSummaryRequestIsRetriedSmallerAndTheFoldLands(t *testing.T) {
	p := &overflowingProvider{limitChars: 150_000}
	a := New(p, tool.NewRegistry(), largeFoldableSession(), Options{ContextWindow: 200_000, CompactRatio: 0.3, RecentKeep: 2, ArchiveDir: testenv.TempDir(t)}, event.Discard)
	if err := prepareContext(context.Background(), a, "auto"); err != nil {
		t.Fatalf("the fold failed after a recoverable refusal: %v", err)
	}
	p.mu.Lock()
	sizes := append([]int(nil), p.sizes...)
	p.mu.Unlock()
	if len(sizes) < 2 {
		t.Fatalf("summary requests = %v, want a refusal and then a smaller request", sizes)
	}
	for i := 1; i < len(sizes); i++ {
		if sizes[i] >= sizes[i-1] {
			t.Fatalf("request %d (%d chars) is not smaller than the refused one (%d)", i, sizes[i], sizes[i-1])
		}
	}
	if sizes[len(sizes)-1] > p.limitChars {
		t.Fatalf("the last request (%d chars) is still over what the provider takes (%d)", sizes[len(sizes)-1], p.limitChars)
	}
	if !strings.Contains(latestDigest(a.sess.win.compactionState.Projection.Messages), "kept the facts") {
		t.Fatal("no digest was installed after the retry")
	}
}

// A provider that refuses every size is a failure with its own class, not a hang.
func TestASummaryRefusedAtEverySizeFailsAsInputTooLarge(t *testing.T) {
	p := &overflowingProvider{limitChars: 100}
	a := New(p, tool.NewRegistry(), largeFoldableSession(), Options{ContextWindow: 200_000, CompactRatio: 0.3, RecentKeep: 2, ArchiveDir: testenv.TempDir(t)}, event.Discard)
	_ = prepareContext(context.Background(), a, "auto")
	p.mu.Lock()
	tried := len(p.sizes)
	p.mu.Unlock()
	if tried < 2 || tried > 4 {
		t.Fatalf("%d summary requests for a provider that refuses everything, want a few smaller ones and then a stop", tried)
	}
	r := a.sess.win.compactionState.LastReceipt
	if r == nil || r.Status != "failed" || r.Code != string(FailSummaryInputTooLarge) {
		t.Fatalf("receipt = %+v, want a failed summary with code %s", r, FailSummaryInputTooLarge)
	}
	if len(a.sess.win.compactionState.Projection.Messages) != 0 {
		t.Fatal("a refused fold installed a projection")
	}
}
