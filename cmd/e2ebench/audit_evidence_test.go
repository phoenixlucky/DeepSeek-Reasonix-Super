package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// writeKilledAgentStub writes a stand-in for the reasonix binary that leaves
// exactly what a killed child leaves behind: no final metrics record, but the
// in-flight snapshot the metrics sink writes every two seconds.
func writeKilledAgentStub(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "stub-agent.sh")
	script := "#!/bin/sh\n" +
		"metrics=\"\"\n" +
		"prev=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--metrics\" ]; then metrics=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"printf '%s' '{\"prompt_tokens\":1200,\"completion_tokens\":80,\"steps\":9," +
		"\"cost\":0.0123,\"currency\":\"USD\",\"complete\":false}' > \"$metrics.partial\"\n" +
		"exit 137\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func killedRun(t *testing.T) result {
	t.Helper()
	requireShellStub(t)
	work := testenv.TempDir(t)
	tk := task{ID: "stub-task", Prompt: "do the thing"}
	r := result{task: tk}
	cfg := suiteConfig{bin: writeKilledAgentStub(t, work)}
	if err := runSegments(context.Background(), cfg, tk, work, "", nil, &r); err == nil {
		t.Fatal("the stub exits 137; a leg that died must be reported as failed")
	}
	return r
}

// writeSilentAgentStub writes a stand-in that leaves neither a final record nor
// a snapshot: what it spent is simply gone, so nothing can bound it.
func writeSilentAgentStub(t *testing.T, dir string) string {
	t.Helper()
	requireShellStub(t)
	path := filepath.Join(dir, "silent-agent.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

// A run recovered from its snapshot is still accounted: its numbers are real and
// stop at the last write, which is exactly what Partial says. Marking it
// Unaccounted would drop them from every total and hide the lower bound.
func TestSuiteSidecarRecoveredRunStaysAccountedAsALowerBound(t *testing.T) {
	r := killedRun(t)
	if r.Unaccounted || !r.Partial {
		t.Fatalf("a run recovered from its snapshot was reported as unmeasured:\n"+
			"  partial=%v unaccounted=%v, want partial=true unaccounted=false", r.Partial, r.Unaccounted)
	}
	stats := gatherSuiteStats([]result{r})
	if stats.unaccounted != 0 || stats.accounted != 1 || stats.partial != 1 {
		t.Fatalf("a recovered run changed category:\n"+
			"  unaccounted=%d accounted=%d partial=%d", stats.unaccounted, stats.accounted, stats.partial)
	}
	if stats.pTok != 1200 || stats.cTok != 80 || stats.cost != 0.0123 {
		t.Fatalf("the recovered numbers never reached the aggregates:\n"+
			"  tokens=%d/%d cost=%.4f, want 1200/80 at 0.0123", stats.pTok, stats.cTok, stats.cost)
	}
	if out := renderBody([]result{r}); !strings.Contains(out, "contributed partial accounting") {
		t.Fatalf("the lower-bound hint is unreachable:\n%s", out)
	}
}

// A leg that leaves neither record has no surviving evidence at all, so the run
// is unmeasured and must stay out of the totals rather than count as zero.
func TestSuiteRunWithNoRecordAtAllIsUnaccounted(t *testing.T) {
	work := testenv.TempDir(t)
	tk := task{ID: "silent-task", Prompt: "do the thing"}
	r := result{task: tk}
	cfg := suiteConfig{bin: writeSilentAgentStub(t, work)}
	if err := runSegments(context.Background(), cfg, tk, work, "", nil, &r); err != nil {
		t.Fatalf("the stub exits 0; a silent leg must not fail the run: %v", err)
	}
	if !r.Unaccounted || r.Partial {
		t.Fatalf("a run that left no record at all was not marked unmeasured:\n"+
			"  partial=%v unaccounted=%v, want partial=false unaccounted=true", r.Partial, r.Unaccounted)
	}
	stats := gatherSuiteStats([]result{r})
	if stats.unaccounted != 1 || stats.accounted != 0 {
		t.Fatalf("a record-less run was counted as accounted:\n"+
			"  unaccounted=%d accounted=%d", stats.unaccounted, stats.accounted)
	}
	if stats.pTok != 0 || stats.cTok != 0 || stats.cost != 0 {
		t.Fatalf("a record-less run contributed numbers: tokens=%d/%d cost=%.4f",
			stats.pTok, stats.cTok, stats.cost)
	}
	if out := renderBody([]result{r}); !strings.Contains(out, "Accounting incomplete for 1 of 1 instances") {
		t.Fatalf("the record-less run was not disclosed as unaccounted:\n%s", out)
	}
}

// The killed child did leave its numbers: the metrics sink snapshots in flight
// into <metrics>.partial. Swebench mode reads that sidecar; the suite path must
// do the same rather than let the directory removal destroy the evidence.
func TestSuiteRunRecoversTheInFlightSnapshotItsChildLeft(t *testing.T) {
	r := killedRun(t)
	if r.PromptTokens != 1200 || r.CompletionTokens != 80 {
		t.Fatalf("the child's in-flight snapshot was dropped:\n"+
			"  prompt_tokens=%d completion_tokens=%d cost=%.4f partial=%v unaccounted=%v\n"+
			"  want the 1200/80 the <metrics>.partial sidecar carries",
			r.PromptTokens, r.CompletionTokens, r.Cost, r.Partial, r.Unaccounted)
	}
}

// The meter-vs-harness divergence line divides one side by the other. Whether a
// run has a harness side is the suite's Unaccounted state, not the token values:
// a run with no record at all has no side, and pricing it against the meter
// invents a divergence and blames the harness for it.
func TestMeterDivergenceDoesNotPriceAMetricslessRun(t *testing.T) {
	r := result{task: task{ID: "killed"}, Unaccounted: true,
		Meter: &meterUsage{PromptTokens: 1200, CompletionTokens: 80}}
	got := renderMeterAccounting([]result{r})
	if strings.Contains(got, "self-report divergence") {
		t.Fatalf("a run with no harness-side number was priced as a self-report divergence:\n%s", got)
	}
}

// A persisted record that says zero tokens is a measurement of zero, not an
// absence: the harness side exists, so it belongs on both sides of the ratio
// even when the number it reports disagrees with the meter.
func TestMeterAccountingKeepsAPersistedZeroOnBothSides(t *testing.T) {
	r := result{task: task{ID: "zero"}, Meter: &meterUsage{PromptTokens: 1200, CompletionTokens: 80}}
	got := renderMeterAccounting([]result{r})
	if !strings.Contains(got, "-100.0% (harness 0 vs meter 1,280 over 1 runs)") {
		t.Fatalf("a real zero-token measurement was dropped as if it were missing:\n%s", got)
	}
}

func TestCorrectBoundaryMediansExcludeRunsWithNoRecordedSplit(t *testing.T) {
	cp := []checkpoint{{Seq: 1, ElapsedMs: 1_000}, {Seq: 2, ElapsedMs: 2_000, Pass: true}}
	measured := result{
		task: task{ID: "measured"}, Passed: true, WallMs: 200_000, Checkpoints: cp,
		FirstCorrectMs: 50_000, PostSolveWasteMs: 150_000,
		MutationsBeforeCorrect: 1, RoundsBeforeCorrect: 3, RoundsAfterCorrect: 4,
		CallsAfterCorrect: 5, VerifyAfterCorrect: 2,
		Trajectory: &trajectorySummary{ModelRounds: 8},
	}
	// the same run twice, but -trajectories was omitted: no digest, so no split
	// was ever taken and the before/after tallies stay at their zero value.
	unmeasured := result{
		task: task{ID: "unmeasured"}, Passed: true, WallMs: 200_000, Checkpoints: cp,
		FirstCorrectMs: 50_000, PostSolveWasteMs: 150_000,
		MutationsBeforeCorrect: 1,
	}
	unmeasured2 := unmeasured
	unmeasured2.task = task{ID: "unmeasured-2"}
	// The audit read the rendered line as plain text; the renderer emits
	// markdown emphasis around each label, so compare with it stripped.
	const want = "**rounds before** 3 · **rounds after** 4"
	got := renderSolveProfiles([]result{measured, unmeasured, unmeasured2})
	if !strings.Contains(got, want) {
		t.Fatalf("medians folded in a run whose trajectory was never split:\n%s\n"+
			"want %q (the measured run only)", got, want)
	}
}

// Control (a): when every run recorded its split, the exclusion must not touch
// a single number — the medians stay the measured ones.
func TestCorrectBoundaryMediansAllMeasuredUnchanged(t *testing.T) {
	cp := []checkpoint{{Seq: 1, ElapsedMs: 1_000}, {Seq: 2, ElapsedMs: 2_000, Pass: true}}
	mk := func(id string, before, after, verify, mutations int) result {
		return result{
			task: task{ID: id}, Passed: true, WallMs: 200_000, Checkpoints: cp,
			FirstCorrectMs: 50_000, PostSolveWasteMs: 150_000,
			RoundsBeforeCorrect: before, RoundsAfterCorrect: after,
			VerifyAfterCorrect: verify, MutationsBeforeCorrect: mutations,
			Trajectory: &trajectorySummary{ModelRounds: 8},
		}
	}
	got := renderSolveProfiles([]result{mk("a", 2, 6, 1, 0), mk("b", 3, 4, 2, 1), mk("c", 4, 5, 3, 2)})
	const want = "**mutations before** 1 · **rounds before** 3 · **rounds after** 5 · **verifications after** 2"
	if !strings.Contains(got, want) {
		t.Fatalf("all-measured medians changed:\n%s\nwant %q", got, want)
	}
}

// Control (b): with -trajectories off no split was ever taken for any run, so
// there is nothing to average. Printing zeros would publish them as
// observations; the report must say the boundary is unmeasured instead.
func TestCorrectBoundaryWithNoRecordedSplitSaysNothingToAverage(t *testing.T) {
	cp := []checkpoint{{Seq: 1, ElapsedMs: 1_000}, {Seq: 2, ElapsedMs: 2_000, Pass: true}}
	run := func(id string, mutations int) result {
		return result{
			task: task{ID: id}, Passed: true, WallMs: 200_000, Checkpoints: cp,
			FirstCorrectMs: 50_000, PostSolveWasteMs: 150_000,
			MutationsBeforeCorrect: mutations,
		}
	}
	got := renderSolveProfiles([]result{run("a", 1), run("b", 3)})
	if strings.Contains(got, "rounds before") || strings.Contains(got, "verifications after") {
		t.Fatalf("a boundary that was never split printed as a measurement:\n%s", got)
	}
	if !strings.Contains(got, "**mutations before** 3") {
		t.Fatalf("the checkpoint-derived mutation count is measured without a trajectory and must still be printed:\n%s", got)
	}
	if !strings.Contains(got, "nothing to average") {
		t.Fatalf("the report must say the boundary was never measured:\n%s", got)
	}
}
