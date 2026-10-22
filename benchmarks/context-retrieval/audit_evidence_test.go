package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runBlockedBatch runs one dry index batch where every arm's runOne fails
// before any model turn: the task's own directory is a regular file, so the
// fixture cannot be built. In production the same early return happens when the
// fixture build, the session load, or the answer-on-disk guard fails.
func runBlockedBatch(t *testing.T) string {
	t.Helper()
	tk := indexTasks()[0]
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, tk.ID), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runExperiment("run-index", experimentIndex, root, true, []contextTask{tk}); code != 0 {
		t.Fatalf("runExperiment exit = %d, want 0 (every run failed, but the batch finished)", code)
	}
	return root
}

func readResultLines(t *testing.T, root string) ([]contextMetrics, []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var runs []contextMetrics
	var lines []string
	for text := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		var m contextMetrics
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Fatalf("results.jsonl: %v", err)
		}
		runs = append(runs, m)
		lines = append(lines, text)
	}
	return runs, lines
}

// A run that failed before the model turn produced no measurement. Recording it
// as a zero-valued contextMetrics puts it in the funnel's Scored denominator,
// in "Where it broke", and in the dose curve as a real zero.
func TestBatchDoesNotRecordAFailedRunAsAResult(t *testing.T) {
	root := runBlockedBatch(t)
	runs, lines := readResultLines(t, root)
	if len(runs) == 0 {
		t.Fatal("no runs recorded at all")
	}
	for i, m := range runs {
		if m.Task == "" || m.Arm == "" || m.FailureStage == "" {
			t.Fatalf("results.jsonl line %d records a run that never ran as a result "+
				"(task=%q arm=%q failure_stage=%q):\n  %s", i+1, m.Task, m.Arm, m.FailureStage, lines[i])
		}
	}
}

// The manifest is the artefact that says which runs counted, so a failed run
// must not be added to CompletedRuns or to the task id list.
func TestBatchManifestDoesNotReportFailedRunsAsCompleted(t *testing.T) {
	root := runBlockedBatch(t)
	var manifest batchManifest
	data, err := os.ReadFile(filepath.Join(root, "batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, id := range manifest.TaskIDs {
		if id == "" {
			t.Fatalf("manifest lists an empty task id: task_ids=%q (completed=%d planned=%d)",
				manifest.TaskIDs, manifest.CompletedRuns, manifest.PlannedRuns)
		}
	}
	if manifest.CompletedRuns == manifest.PlannedRuns {
		t.Fatalf("manifest reports every planned run completed although none produced evidence: "+
			"completed=%d planned=%d task_ids=%q", manifest.CompletedRuns, manifest.PlannedRuns, manifest.TaskIDs)
	}
}
