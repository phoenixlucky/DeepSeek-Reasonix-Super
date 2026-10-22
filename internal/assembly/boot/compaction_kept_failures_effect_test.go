package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/session/control"
)

const (
	failureRunsPerSession = 60
	failureCeiling        = 6
)

// failingTurnProvider answers every user turn with one failing command whose
// output is tagged by turn, then closes the turn; it writes a digest when asked
// to summarize.
type failingTurnProvider struct {
	mu    sync.Mutex
	reqs  []provider.Request
	turns int
}

func (p *failingTurnProvider) Name() string { return "boot-failing-turn" }

func (p *failingTurnProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	summary := req.Summary
	last := req.Messages[len(req.Messages)-1]
	newTurn := !summary && last.Role == provider.RoleUser
	if newTurn {
		p.turns++
	}
	turn := p.turns
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 3)
	switch {
	case summary:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "## Goal\n- keep running the check"}
	case newTurn:
		raw, _ := json.Marshal(map[string]any{"command": fmt.Sprintf("echo FAILRUN-%d %s; exit 1", turn, strings.Repeat("detail ", 300))})
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: fmt.Sprintf("t%d", turn), Name: "bash", Arguments: string(raw)}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "noted"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *failingTurnProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// A long session of failing commands folds many times. Retained failures ride
// into each next fold, so what the model is sent must stay bounded by the
// failure budget and the recent tail, not grow with the number of folds.
func TestEffectRetainedFailuresDoNotAccumulateAcrossFolds(t *testing.T) {
	t.Parallel()
	home := statedBootHome(t)
	dir := testenv.TempDir(t)
	kind := uniqueKind("boot-failing-turn")
	rec := &failingTurnProvider{}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeUserConfigAt(t, home, "[sandbox]\nbash = \"off\"\n")
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
compact_ratio = 0.8
recent_keep = 2

[codegraph]
enabled = false

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
context_window = 32000
`)
	approveWorkspaceAt(t, home, dir)

	ctrl, err := Build(context.Background(), Options{Home: home, WorkspaceRoot: dir})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	ctrl.SetToolApprovalMode(control.ToolApprovalYolo)
	ctrl.SetSessionPath(filepath.Join(dir, ".reasonix", "sessions", "failing-turns.jsonl"))
	for i := range failureRunsPerSession {
		if err := ctrl.Run(context.Background(), fmt.Sprintf("run the check again (%d)", i)); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}

	count := func(req provider.Request) (failures, newest int) {
		for _, m := range req.Messages {
			if m.Role != provider.RoleTool || !strings.Contains(m.Content, "FAILRUN-") {
				continue
			}
			failures++
			var n int
			if _, err := fmt.Sscanf(m.Content[strings.Index(m.Content, "FAILRUN-"):], "FAILRUN-%d", &n); err == nil && n > newest {
				newest = n
			}
		}
		return failures, newest
	}
	peak, newestSeen, folds := 0, 0, 0
	afterFold := false
	for _, req := range rec.requests() {
		if len(req.Messages) == 0 {
			continue
		}
		if req.Summary {
			afterFold = true
			folds++
			continue
		}
		failures, newest := count(req)
		newestSeen = max(newestSeen, newest)
		if afterFold {
			peak = max(peak, failures)
			afterFold = false
		}
	}
	t.Logf("%d folds; at most %d failed results right after one, newest FAILRUN-%d", folds, peak, newestSeen)
	if folds < 5 {
		t.Fatalf("only %d folds ran; the session is too short to show accumulation", folds)
	}
	if newestSeen != failureRunsPerSession {
		t.Errorf("the newest failure FAILRUN-%d never reached the model (newest seen: %d)", failureRunsPerSession, newestSeen)
	}
	if peak > failureCeiling {
		t.Errorf("the request after a fold carried %d failed results after %d turns; retained failures are accumulating across folds", peak, failureRunsPerSession)
	}
}
