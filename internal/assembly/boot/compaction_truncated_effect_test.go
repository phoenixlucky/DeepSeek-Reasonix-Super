package boot

import (
	"context"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

type truncatingSummarizer struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *truncatingSummarizer) Name() string { return "boot-truncating-summarizer" }

func (p *truncatingSummarizer) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 3)
	if req.Summary {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "## partial"}
		ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{FinishReason: "length"}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: strings.Repeat("work output line with detail. ", 400)}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *truncatingSummarizer) summarizerRequests() (out []provider.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.reqs {
		if r.Summary {
			out = append(out, r)
		}
	}
	return out
}

func (p *truncatingSummarizer) turnRequests() (out []provider.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.reqs {
		if !r.Summary {
			out = append(out, r)
		}
	}
	return out
}

type collectSink struct {
	mu sync.Mutex
	ev []event.Event
}

func (s *collectSink) Emit(e event.Event) { s.mu.Lock(); s.ev = append(s.ev, e); s.mu.Unlock() }

// truncatingFixtureChars is the first-request size this fixture is padded to:
// from 30000 to 40000 characters it makes the same 6 attempts, and its first
// request is about 27200 unpadded, so either side has room to move.
const truncatingFixtureChars = 35000

func buildTruncatingSession(t *testing.T) (*truncatingSummarizer, *collectSink, func(string)) {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	probe, _, run, stop := startTruncatingSession(t, dir, "probe", "")
	run("keep going")
	first := probe.firstRequest()
	stop()
	rec, sink, run, _ := startTruncatingSession(t, dir, "fixture", promptPadding(t, first, truncatingFixtureChars))
	return rec, sink, run
}

func startTruncatingSession(t *testing.T, dir, role, padding string) (*truncatingSummarizer, *collectSink, func(string), func()) {
	t.Helper()
	rec := &truncatingSummarizer{}
	kind := "boot-truncating-" + role + "-" + t.Name()
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE`+padding+`"
compact_ratio = 0.5

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
context_window = 32500
effort = "high"
`)
	approveWorkspace(t, dir)
	sink := &collectSink{}
	ctrl, err := Build(context.Background(), Options{Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var once sync.Once
	stop := func() { once.Do(func() { ctrl.Close() }) }
	t.Cleanup(stop)
	return rec, sink, func(p string) { _ = ctrl.Run(context.Background(), p) }, stop
}

func (p *truncatingSummarizer) firstRequest() provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.reqs) == 0 {
		return provider.Request{}
	}
	return p.reqs[0]
}

// The digest is a reasoning request under the session's own effort: its
// reasoning tokens spend the same output cap the digest has to fit in.
func TestEffectSummarizerRequestDoesNotInheritSessionEffort(t *testing.T) {
	rec, _, run := buildTruncatingSession(t)
	for range 8 {
		run("keep going")
	}
	reqs := rec.summarizerRequests()
	if len(reqs) == 0 {
		t.Fatal("fixture never reached a summarizer request")
	}
	if reqs[0].EffortOverride != "low" {
		t.Fatalf("summarizer EffortOverride = %q, want low (MaxTokens=%d)", reqs[0].EffortOverride, reqs[0].MaxTokens)
	}
	turns := rec.turnRequests()
	if len(turns) == 0 {
		t.Fatal("fixture made no normal turn request")
	}
	for i, r := range turns {
		if r.EffortOverride != "" {
			t.Fatalf("normal turn %d carries EffortOverride %q; only the digest may", i, r.EffortOverride)
		}
	}
}

// A truncated digest must reach the user as its typed code on the automatic
// path too, and must not leave automatic compaction blocked for good.
func TestEffectTruncatedSummaryOnPressureKeepsCodeAndRetries(t *testing.T) {
	rec, sink, run := buildTruncatingSession(t)
	for range 10 {
		run("keep going")
	}
	if got := requestChars(rec.firstRequest()); got != truncatingFixtureChars {
		t.Fatalf("the fixture's first request is %d characters, want %d: the padding did not reach it", got, truncatingFixtureChars)
	}
	var code string
	for _, e := range sink.ev {
		if e.Kind == event.CompactionDone && e.Compaction.Messages == 0 {
			code = e.Compaction.Code
			break
		}
	}
	if code != "summary_truncated" {
		t.Fatalf("pressure card code = %q, want summary_truncated", code)
	}
	reqs := rec.summarizerRequests()
	if len(reqs)%2 != 0 {
		t.Fatalf("%d summarizer requests; every cut summary is tried at its cap and once more, so they come in pairs", len(reqs))
	}
	for i := 0; i < len(reqs); i += 2 {
		if reqs[i+1].MaxTokens <= reqs[i].MaxTokens {
			t.Fatalf("attempt %d retried at cap %d after %d; the retry must have more room", i/2, reqs[i+1].MaxTokens, reqs[i].MaxTokens)
		}
	}
	attempts := len(reqs) / 2
	if attempts > 6 {
		t.Fatalf("automatic compaction made %d failed summaries over 10 turns; retries must be bounded by input growth", attempts)
	}
	if attempts < 2 {
		t.Fatalf("automatic compaction made %d summary attempt(s) over 10 turns; one truncation blocked it for the generation", attempts)
	}
}
