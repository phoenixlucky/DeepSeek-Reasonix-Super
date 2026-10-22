package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func trialResult(id string, trial int, passed bool) result {
	return result{task: task{ID: id}, Trial: trial, Passed: passed}
}

// pass^k is the share of tasks that passed every trial, pass@k the share that
// passed any, and mean pass@1 the share of all trials that passed. A task that
// passed some trials is named, since it is the one worth reading.
func TestTrialsLineReportsReliabilityAcrossTrials(t *testing.T) {
	results := []result{
		trialResult("steady", 1, true), trialResult("steady", 2, true), trialResult("steady", 3, true),
		trialResult("flaky", 1, true), trialResult("flaky", 2, false), trialResult("flaky", 3, true),
		trialResult("broken", 1, false), trialResult("broken", 2, false), trialResult("broken", 3, false),
	}
	line := trialsLine(results)
	for _, want := range []string{
		"3 trials, 3/3 tasks complete",
		"**pass^3** 33% (1/3 complete)",
		"**pass@3** 67% (2/3 complete)",
		"**mean pass@1** 56% (5/9)",
		"`flaky` 2/3",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("missing %q in:\n%s", want, line)
		}
	}
	if strings.Contains(line, "`steady`") || strings.Contains(line, "`broken`") {
		t.Fatalf("only a task with mixed outcomes is flaky:\n%s", line)
	}
}

// pass^k is the chance all k trials pass, which a task that ran fewer has no
// value for. A trial the budget skipped is not a failure, so such a task is
// left out of pass^k and pass@k and named, and its trials still count toward
// mean pass@1.
func TestTrialsLineKeepsATaskThatRanFewerTrialsOutOfPassK(t *testing.T) {
	for _, bPassed := range []bool{true, false} {
		results := []result{
			trialResult("a", 1, true), trialResult("a", 2, true),
			trialResult("b", 1, bPassed), {task: task{ID: "b"}, Trial: 2, Skipped: true},
		}
		line := trialsLine(results)
		for _, want := range []string{"1/2 tasks complete", "**pass^2** 100% (1/1 complete)", "**pass@2** 100% (1/1 complete)", "**Incomplete** (fewer than 2 trials, not in pass^2 or pass@2): `b` 1/2"} {
			if !strings.Contains(line, want) {
				t.Fatalf("bPassed=%v: missing %q in:\n%s", bPassed, want, line)
			}
		}
	}
}

func TestTrialsLineHasNoPassKWhenNoTaskRanEveryTrial(t *testing.T) {
	results := []result{trialResult("a", 1, true), trialResult("a", 3, true), trialResult("b", 1, true), trialResult("b", 2, true)}
	line := trialsLine(results)
	if !strings.Contains(line, "**pass^3** n/a (no task ran 3 trials)") || !strings.Contains(line, "**pass@3** n/a") {
		t.Fatalf("line = %s", line)
	}
}

// Without trials the report reads exactly as it did.
func TestTrialsLineIsSilentWithoutTrials(t *testing.T) {
	if got := trialsLine([]result{{task: task{ID: "a"}, Attempt: 1, Passed: true}}); got != "" {
		t.Fatalf("line = %q, want nothing for a run without trials", got)
	}
}

// trajectoryAgent stands in for the reasonix binary and writes a one-record
// trajectory wherever --trajectory points, so a test can see which files a run
// leaves behind.
func trajectoryAgent(t *testing.T) string {
	t.Helper()
	requireShellStub(t)
	bin := filepath.Join(testenv.TempDir(t), "fake-agent")
	script := `#!/usr/bin/env bash
while [ $# -gt 0 ]; do
  case "$1" in
    --trajectory) printf '{"ts":1,"event":{"kind":"turn_done"}}\n' > "$2"; shift 2;;
    --metrics) printf '{"steps":1,"complete":true}' > "$2"; shift 2;;
    *) shift;;
  esac
done
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// Each trial records its own trajectory: a shared name would leave only the
// last trial's file, and every digest of the earlier ones would read it.
func TestTrialsKeepOneTrajectoryPerTrial(t *testing.T) {
	dir, taskDir := testenv.TempDir(t), testenv.TempDir(t)
	if err := os.WriteFile(filepath.Join(taskDir, "prompt.md"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := suiteConfig{bin: trajectoryAgent(t), trials: 2, trajDir: dir}
	total := 0
	results := runTrials(cfg, task{ID: "demo", Prompt: "p", MaxSteps: 4, TimeoutSec: 60, dir: taskDir}, &total)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	got, err := filepath.Glob(filepath.Join(dir, "*.trajectory.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		got[i] = filepath.Base(got[i])
	}
	slices.Sort(got)
	want := []string{"demo.trial-1.trajectory.jsonl", "demo.trial-2.trajectory.jsonl"}
	if !slices.Equal(got, want) {
		t.Fatalf("trajectory files = %v, want %v", got, want)
	}
	for _, r := range results {
		if r.Trajectory == nil || filepath.Base(r.Trajectory.Path) != want[r.Trial-1] {
			t.Errorf("trial %d digest = %+v, want it read from %s", r.Trial, r.Trajectory, want[r.Trial-1])
		}
	}
}

// A single trial keeps the name every reader already knows.
func TestSingleTrialKeepsTheTaskTrajectoryName(t *testing.T) {
	if got := trajectoryStem("demo", 1, 1); got != "demo" {
		t.Fatalf("stem = %q, want demo", got)
	}
	if got := trajectoryStem("demo", 0, 0); got != "demo" {
		t.Fatalf("stem = %q, want demo", got)
	}
}

// Readers that need the task behind a file strip the trial and segment marks.
func TestTrajectoryTaskIDReadsEveryFileShape(t *testing.T) {
	for name, want := range map[string]string{
		"demo.trajectory.jsonl":              "demo",
		"demo.trial-2.trajectory.jsonl":      "demo",
		"demo.seg3.trajectory.jsonl":         "demo",
		"demo.trial-2.seg3.trajectory.jsonl": "demo",
		"a.b.trial-10.trajectory.jsonl":      "a.b",
	} {
		if got := trajectoryTaskID(filepath.Join("d", name)); got != want {
			t.Errorf("trajectoryTaskID(%s) = %q, want %q", name, got, want)
		}
	}
}
