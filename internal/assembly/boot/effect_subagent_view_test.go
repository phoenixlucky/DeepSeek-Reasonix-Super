package boot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/runtime/delegation"
)

// A background sub-agent is readable through the controller while it runs, from
// the real assembly: the child's file is written only when it ends, so what the
// reader sees mid-run has to come from the live run.
func TestEffectARunningSubagentIsReadableThroughTheController(t *testing.T) {
	home := isolateConfigHome(t)
	_ = os.MkdirAll(filepath.Join(home, ".reasonix", "sessions"), 0o755)
	dir := robustTempDir(t)
	t.Chdir(dir)
	prov := &slowChildProvider{target: filepath.Join(dir, "parent.txt"), started: make(chan struct{}), release: make(chan struct{})}
	slowChildCurrent.Store(prov)
	slowChildRegistered.Do(registerSlowChild)
	if err := os.MkdirAll(filepath.Dir(config.UserConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "boot-effect-slow-child"
model = "x"
`)
	approveWorkspace(t, dir)
	sink := &childReceiptSink{done: make(chan struct{})}
	ctrl, err := Build(context.Background(), Options{Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	_ = os.WriteFile(filepath.Join(home, ".reasonix", "sessions", "o.execution.jsonl"), nil, 0o600)
	ctrl.SetSessionPath(filepath.Join(dir, ".reasonix", "sessions", "o.jsonl"))
	ctrl.SetBypass(true)
	ctrl.Submit("go")
	select {
	case <-sink.done:
	case <-time.After(30 * time.Second):
		t.Fatal("turn did not finish")
	}

	runs, err := ctrl.SubagentRuns()
	if err != nil || len(runs) != 1 || runs[0].State != delegation.RunRunning {
		t.Fatalf("runs while the child is held = %+v err=%v, want one running run", runs, err)
	}
	live, err := ctrl.SubagentTranscript(runs[0].Ref, 0, 0)
	if err != nil || !live.Live || !transcriptSays(live, "slow edits") {
		t.Fatalf("live transcript = %+v err=%v, want the child's task in a live read", live, err)
	}

	close(prov.release)
	deadline := time.Now().Add(30 * time.Second)
	for {
		got, _ := ctrl.SubagentRuns()
		if len(got) == 1 && got[0].State == delegation.RunCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child never reached completed: %+v", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
	done, err := ctrl.SubagentTranscript(runs[0].Ref, 0, 0)
	if err != nil || done.Live || !transcriptSays(done, "child done") {
		t.Fatalf("finished transcript = %+v err=%v, want the stored answer and not live", done, err)
	}
}

func transcriptSays(tr delegation.RunTranscript, text string) bool {
	for _, m := range tr.Messages {
		if strings.Contains(m.Content, text) {
			return true
		}
	}
	return false
}
