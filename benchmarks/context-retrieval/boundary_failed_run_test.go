package main

import (
	"strings"
	"testing"
)

// boundaryRuns builds the input reportBoundaries reads: one scored run for an
// index task's cue side, and one failed run for another index task's cue side.
// The runs are constructed because the report is a pure function of the slice;
// the batch path that produces them is covered by the failed-batch tests.
func boundaryRuns(t *testing.T) (all []contextMetrics, scoredTask, failedTask string) {
	t.Helper()
	index := indexTasks()
	if len(index) < 2 {
		t.Fatalf("need at least two index tasks, have %d", len(index))
	}
	scored, failed := index[0], index[1]
	cue, noCue := boundaryPair(scored.CueTier)
	if cue == "" || noCue == "" {
		t.Fatalf("task %s has an empty boundary pair (%q, %q)", scored.ID, cue, noCue)
	}
	failedCue, _ := boundaryPair(failed.CueTier)
	if failedCue == "" {
		t.Fatalf("task %s has no cue side", failed.ID)
	}
	all = []contextMetrics{
		{Task: scored.ID, Arm: "index-" + cue, AnswerRecovered: true, SearchCalls: 1, RecallReturnedTokens: 318},
		{Task: scored.ID, Arm: "index-" + noCue, AnswerRecovered: false},
		{Task: failed.ID, Arm: "index-" + failedCue, Failed: true, FailureStage: stageNoEvidence},
	}
	return all, scored.ID, failed.ID
}

// TestBoundaryReportExcludesFailedRuns: a run that never produced a transcript
// must not appear in the cue-boundary table, which was the last reporting exit
// that still folded it in as a zero measurement.
func TestBoundaryReportExcludesFailedRuns(t *testing.T) {
	all, scoredTask, failedTask := boundaryRuns(t)
	out := captureStdout(t, func() { reportBoundaries(all) })
	if strings.Contains(out, failedTask) {
		t.Fatalf("the boundary table printed the failed task %q as a measurement:\n%s", failedTask, out)
	}
	if !strings.Contains(out, scoredTask) {
		t.Fatalf("the boundary table dropped the scored task %q, so the exclusion is not specific to failures:\n%s", scoredTask, out)
	}
	if !strings.Contains(out, "cue-present") || !strings.Contains(out, "cue-absent") {
		t.Fatalf("the pooled comparison disappeared:\n%s", out)
	}
}

// TestBoundaryReportSaysNothingToCompareWhenEveryRunFailed: with no scored run
// the report must say so, rather than printing a comparison of zeros.
func TestBoundaryReportSaysNothingToCompareWhenEveryRunFailed(t *testing.T) {
	all, _, _ := boundaryRuns(t)
	failedOnly := make([]contextMetrics, 0, len(all))
	for _, m := range all {
		m.Failed = true
		m.FailureStage = stageNoEvidence
		failedOnly = append(failedOnly, m)
	}
	out := captureStdout(t, func() { reportBoundaries(failedOnly) })
	if !strings.Contains(out, "scored=0") || !strings.Contains(out, "failed=") {
		t.Fatalf("an all-failed batch did not report scored=0 with a failed count:\n%s", out)
	}
	if strings.Contains(out, "cue-present") || strings.Contains(out, "cue-absent") {
		t.Fatalf("an all-failed batch still printed a pooled comparison of zeros:\n%s", out)
	}
}
