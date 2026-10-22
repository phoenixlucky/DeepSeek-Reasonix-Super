package agent

import (
	"context"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

type effortProbe struct {
	levels  []string
	efforts []string
}

func (p *effortProbe) Name() string { return "effort-probe" }

func (p *effortProbe) RequestEfforts() []string { return p.levels }

func (p *effortProbe) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.efforts = append(p.efforts, req.EffortOverride)
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// A session that changed depth carries it on the next main request without being
// rebuilt, and going back to the built depth carries nothing.
func TestARequestScopedEffortRidesTheNextMainRequest(t *testing.T) {
	p := &effortProbe{levels: []string{"low", "high", "max"}}
	a := New(p, tool.NewRegistry(), sessionstore.NewSession("sys"), Options{}, event.Discard)
	if got := a.RequestEffortLevels(); len(got) != 3 {
		t.Fatalf("levels = %v, want the provider's three", got)
	}
	run := func() {
		t.Helper()
		if err := a.Run(context.Background(), "hello"); err != nil {
			t.Fatal(err)
		}
	}
	run()
	a.SetRequestEffort("High ")
	run()
	a.SetRequestEffort("")
	run()
	want := []string{"", "high", ""}
	if len(p.efforts) != len(want) {
		t.Fatalf("requests carried %v, want %v", p.efforts, want)
	}
	for i := range want {
		if p.efforts[i] != want[i] {
			t.Fatalf("request %d carried %q, want %q (all: %v)", i, p.efforts[i], want[i], p.efforts)
		}
	}
}

// A provider that takes only the effort it was built with exposes no levels.
func TestAProviderWithoutRequestEffortsExposesNoLevels(t *testing.T) {
	a := New(&mockProviderNoEffort{}, tool.NewRegistry(), sessionstore.NewSession("sys"), Options{}, event.Discard)
	if got := a.RequestEffortLevels(); len(got) != 0 {
		t.Fatalf("levels = %v, want none", got)
	}
}

type mockProviderNoEffort struct{}

func (*mockProviderNoEffort) Name() string { return "plain" }
func (*mockProviderNoEffort) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk)
	close(ch)
	return ch, nil
}
