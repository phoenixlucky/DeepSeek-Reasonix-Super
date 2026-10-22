package boot

import (
	"context"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/eventwire"
	"reasonix/internal/contract/provider"
)

// slowFirstProvider holds back its first output, the way a provider that is
// still queueing or prefilling does.
type slowFirstProvider struct{ wait time.Duration }

func (p *slowFirstProvider) Name() string { return "boot-first-token" }

func (p *slowFirstProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 2)
	go func() {
		time.Sleep(p.wait)
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
		ch <- provider.Chunk{Type: provider.ChunkDone}
		close(ch)
	}()
	return ch, nil
}

// The time to first token is measured by the host and has to reach the frontend
// as a committed round's own field, not be left for each client to guess from
// its clock.
func TestEffectCommittedRoundCarriesTimeToFirstToken(t *testing.T) {
	t.Parallel()
	home := statedBootHome(t)
	dir := testenv.TempDir(t)
	const wait = 120 * time.Millisecond
	kind := "boot-first-token-" + t.Name()
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return &slowFirstProvider{wait: wait}, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	approveWorkspaceAt(t, home, dir)

	var mu sync.Mutex
	var commits []eventwire.StreamAttempt
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind != event.StreamAttempt {
			return
		}
		w := eventwire.ToWire(e)
		if w.StreamAttempt != nil && w.StreamAttempt.Action == string(event.StreamAttemptCommit) {
			mu.Lock()
			commits = append(commits, *w.StreamAttempt)
			mu.Unlock()
		}
	})
	ctrl, err := Build(context.Background(), Options{Home: home, WorkspaceRoot: dir, Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "say ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(commits) == 0 {
		t.Fatal("no round was committed")
	}
	got := time.Duration(commits[0].FirstTokenMs) * time.Millisecond
	if got < wait || got > 10*wait {
		t.Fatalf("committed round reports first token after %v, provider held it back %v", got, wait)
	}
}
