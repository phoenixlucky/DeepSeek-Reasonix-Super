package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// blockTask makes one task fail before its first model turn: its own directory
// is a regular file, so buildFixture cannot create the session under it. It is
// the same early exit as a failed fixture build, session load, or
// answer-on-disk guard, and it needs no provider.
func blockTask(t *testing.T, root, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, id), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mixedTaskPair is one task whose runs all fail and one whose runs all succeed.
// Both are drawn from the corpus, so the batch is the real pipeline.
func mixedTaskPair() (blocked, ok contextTask) {
	blocked, ok = indexTasks()[0], indexTasks()[1]
	for _, t := range indexTasks() {
		if t.Role == roleEfficiency && len(armsFor(t)) == 2 {
			ok = t
			break
		}
	}
	return blocked, ok
}

// runBlockedExperiment runs one dry index batch over the given tasks, with the
// named task failing before any model turn, and returns the batch directory, the
// runs, the manifest, and everything the run printed.
func runBlockedExperiment(t *testing.T, tasks []contextTask, blocked ...string) (string, []contextMetrics, batchManifest, string) {
	t.Helper()
	root := t.TempDir()
	for _, id := range blocked {
		blockTask(t, root, id)
	}
	out := captureStdout(t, func() {
		if code := runExperiment("run-index", experimentIndex, root, true, tasks); code != 0 {
			t.Errorf("runExperiment exit = %d, want 0 (the batch finished)", code)
		}
	})
	runs, _ := readResultLines(t, root)
	manifest, _ := readManifest(t, root)
	return root, runs, manifest, out
}

// captureStdout reads everything the function prints. os.Pipe is unbuffered, so
// the reader must run beside the call.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	f()
	os.Stdout = old
	w.Close()
	text := <-done
	r.Close()
	return text
}

// funnelScored reads the "runs=N" column of the named arm's funnel line out of
// the printed tables, so the assertion is on the number a reader sees.
func funnelScored(t *testing.T, out, arm string) int {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != arm || !strings.HasPrefix(fields[1], "runs=") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(fields[1], "runs="))
		if err != nil {
			t.Fatalf("funnel line for %s: %v\n%s", arm, err, line)
		}
		return n
	}
	t.Fatalf("no funnel line for arm %s:\n%s", arm, out)
	return 0
}

// The batch report states both counts on one line, so a reader of the scrollback
// sees the scored set and the failures without counting funnel rows.
func reportCounts(t *testing.T, out string) (scored, failed int) {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.Contains(line, " scored=") || !strings.Contains(line, " failed=") {
			continue
		}
		for f := range strings.FieldsSeq(line) {
			switch {
			case strings.HasPrefix(f, "scored="):
				scored, _ = strconv.Atoi(strings.TrimPrefix(f, "scored="))
			case strings.HasPrefix(f, "failed="):
				failed, _ = strconv.Atoi(strings.TrimPrefix(f, "failed="))
			}
		}
		return scored, failed
	}
	t.Fatalf("no line reports scored= and failed=:\n%s", out)
	return 0, 0
}

// A batch that mixes a task that never ran with one that did must read as its
// successful runs only: the failed task leaves Scored, the funnel, and the
// completed count, and is reported as a failure with its identity instead.
// Every assertion here is a number, not the absence of an empty row.
func TestMixedBatchCountsOnlyTheRunThatProducedEvidence(t *testing.T) {
	blocked, ok := mixedTaskPair()
	_, runs, manifest, out := runBlockedExperiment(t, []contextTask{blocked, ok}, blocked.ID)

	wantScored, wantFailed := len(armsFor(ok)), len(armsFor(blocked))
	if manifest.CompletedRuns != wantScored || manifest.FailedRuns != wantFailed {
		t.Fatalf("manifest completed=%d failed=%d, want %d and %d (planned=%d)",
			manifest.CompletedRuns, manifest.FailedRuns, wantScored, wantFailed, manifest.PlannedRuns)
	}
	if len(runs) != manifest.CompletedRuns+manifest.FailedRuns {
		t.Fatalf("results.jsonl has %d rows but completed=%d failed=%d",
			len(runs), manifest.CompletedRuns, manifest.FailedRuns)
	}
	for _, id := range manifest.TaskIDs {
		if id == blocked.ID {
			t.Fatalf("task_ids lists %s, which produced no evidence: %v", blocked.ID, manifest.TaskIDs)
		}
	}
	scored, failures := 0, 0
	for _, m := range runs {
		if m.Failed {
			failures++
			if m.Task != blocked.ID || m.FailureStage == "" || m.Reason == "" {
				t.Fatalf("failed row lost its identity or stage: %+v", m)
			}
			continue
		}
		scored++
		if m.Task != ok.ID {
			t.Fatalf("a run scored from the wrong task: %+v", m)
		}
	}
	if scored != wantScored || failures != wantFailed {
		t.Fatalf("rows scored=%d failed=%d, want %d and %d", scored, failures, wantScored, wantFailed)
	}
	// Each arm's funnel holds exactly the successful task's runs, and the two
	// scored counts agree with each other and with the manifest.
	total := 0
	for _, arm := range armsFor(ok) {
		total += funnelScored(t, out, arm.name)
	}
	if total != manifest.CompletedRuns {
		t.Fatalf("funnels scored %d runs, manifest completed_runs = %d:\n%s", total, manifest.CompletedRuns, out)
	}
	if gotScored, gotFailed := reportCounts(t, out); gotScored != manifest.CompletedRuns || gotFailed != manifest.FailedRuns {
		t.Fatalf("report says scored=%d failed=%d, manifest says %d and %d",
			gotScored, gotFailed, manifest.CompletedRuns, manifest.FailedRuns)
	}
	if strings.Contains(out, "## Where it broke") && strings.Contains(out, "=-\n") {
		t.Fatalf("the stage table kept an empty stage:\n%s", out)
	}
}

// A batch where every run fails must not read as an empty success: zero scored
// runs, a nonzero failed count, and both stated.
func TestAllFailedBatchReportsZeroScoredAndNonzeroFailed(t *testing.T) {
	blocked, _ := mixedTaskPair()
	_, runs, manifest, out := runBlockedExperiment(t, []contextTask{blocked}, blocked.ID)

	if manifest.CompletedRuns != 0 {
		t.Fatalf("completed_runs = %d, want 0 (nothing produced evidence)", manifest.CompletedRuns)
	}
	if manifest.FailedRuns != manifest.PlannedRuns || manifest.FailedRuns == 0 {
		t.Fatalf("failed_runs = %d, planned_runs = %d: every planned run failed",
			manifest.FailedRuns, manifest.PlannedRuns)
	}
	if len(manifest.TaskIDs) != 0 {
		t.Fatalf("task_ids = %v, want none", manifest.TaskIDs)
	}
	for i, m := range runs {
		if !m.Failed || m.FailureStage == "" || m.Task != blocked.ID {
			t.Fatalf("results.jsonl line %d is not a marked failure: %+v", i+1, m)
		}
	}
	total := 0
	for _, arm := range armsFor(blocked) {
		total += funnelScored(t, out, arm.name)
	}
	if total != 0 {
		t.Fatalf("funnels scored %d runs although nothing ran:\n%s", total, out)
	}
	if gotScored, gotFailed := reportCounts(t, out); gotScored != 0 || gotFailed != manifest.FailedRuns {
		t.Fatalf("report says scored=%d failed=%d, want 0 and %d", gotScored, gotFailed, manifest.FailedRuns)
	}
}

// The control: with nothing blocked, a batch is what it always was. Every run
// is completed with a stage, two identical batches write byte-identical results
// and the same manifest header, and the new failure field stays out of it.
func TestAllSuccessfulBatchKeepsItsArtifacts(t *testing.T) {
	tasks := indexTasks()
	first, runs, manifest, out := runBlockedExperiment(t, tasks)
	second, again, repeat, _ := runBlockedExperiment(t, tasks)
	if manifest.CompletedRuns != len(runs) || manifest.FailedRuns != 0 {
		t.Fatalf("completed=%d failed=%d rows=%d, want every row completed",
			manifest.CompletedRuns, manifest.FailedRuns, len(runs))
	}
	if manifest.CompletedRuns != manifest.PlannedRuns {
		t.Fatalf("completed=%d planned=%d, want all planned", manifest.CompletedRuns, manifest.PlannedRuns)
	}
	for i, m := range runs {
		if m.Failed || m.Task == "" || m.Arm == "" || m.FailureStage == "" {
			t.Fatalf("results.jsonl line %d is not a completed run: %+v", i+1, m)
		}
	}
	if gotScored, gotFailed := reportCounts(t, out); gotScored != manifest.CompletedRuns || gotFailed != 0 {
		t.Fatalf("report says scored=%d failed=%d, want %d and 0", gotScored, gotFailed, manifest.CompletedRuns)
	}
	if !reflect.DeepEqual(runs, again) {
		t.Fatal("two identical successful batches scored differently")
	}
	// Byte-for-byte on the runs. Trajectories carry per-message wall-clock
	// stamps, so they are compared field by field instead.
	if got, want := readFile(t, second, "results.jsonl"), readFile(t, first, "results.jsonl"); got != want {
		t.Fatalf("an unchanged batch wrote different results.jsonl:\n%s\n---\n%s", want, got)
	}
	gotTraj, wantTraj := withoutStamps(t, readFile(t, second, "trajectories.jsonl")),
		withoutStamps(t, readFile(t, first, "trajectories.jsonl"))
	if gotTraj != wantTraj {
		t.Fatalf("an unchanged batch wrote different trajectories:\n%s\n---\n%s", wantTraj, gotTraj)
	}
	repeat.StartedAt, repeat.FinishedAt = manifest.StartedAt, manifest.FinishedAt
	repeat.TrajectoriesSHA256 = manifest.TrajectoriesSHA256
	if !reflect.DeepEqual(repeat, manifest) {
		t.Fatalf("an unchanged batch wrote a different manifest:\n%+v\n---\n%+v", manifest, repeat)
	}
	if repeat.FailedRuns != 0 {
		t.Fatalf("a batch with no failure reports failed_runs=%d", repeat.FailedRuns)
	}
}

// withoutStamps drops the createdAt stamp every message carries, which is the
// one field a dry batch cannot reproduce between two runs.
func withoutStamps(t *testing.T, text string) string {
	t.Helper()
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		stripStamp(row)
		out, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(out)
		b.WriteByte('\n')
	}
	return b.String()
}

func stripStamp(v any) {
	switch typed := v.(type) {
	case map[string]any:
		delete(typed, "createdAt")
		delete(typed, "workDurationMs")
		for _, child := range typed {
			stripStamp(child)
		}
	case []any:
		for _, child := range typed {
			stripStamp(child)
		}
	}
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
