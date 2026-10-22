package main

// Wall population rule: a rate figure divides every run's wall (a killed run
// still spent it); a per-solved cost divides the accounted wall of solved runs.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// reportSolvedPerHour reads the "Solved/hour" number the suite report prints.
func reportSolvedPerHour(t *testing.T, report string) string {
	t.Helper()
	m := regexp.MustCompile(`\*\*Solved/hour\*\* ([\d.]+)`).FindStringSubmatch(report)
	if m == nil {
		t.Fatalf("suite report has no Solved/hour line:\n%s", report)
	}
	return m[1]
}

// compareSolvedPerHour reads the Solved/hour column out of the compare table's
// one arm row.
func compareSolvedPerHour(t *testing.T, report string) string {
	t.Helper()
	for line := range strings.SplitSeq(report, "\n") {
		if !strings.HasPrefix(line, "| `arm` |") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 8 {
			t.Fatalf("compare row has %d cells:\n%s", len(cells), line)
		}
		return strings.TrimSpace(cells[7])
	}
	t.Fatalf("compare table has no arm row:\n%s", report)
	return ""
}

// Solved/hour is a rate, not a per-solved figure: the suite report divides every
// run's wall (a killed run's tokens are unknown, its wall was still spent), so
// aggregateArm must not drop the unaccounted run's wall from that denominator.
func TestCompareSolvedPerHourUsesTheSameWallAsTheReport(t *testing.T) {
	dir := testenv.TempDir(t)
	results := []result{
		{task: task{ID: "ok"}, Passed: true, WallMs: 60_000},
		{task: task{ID: "killed"}, Passed: true, WallMs: 1_800_000, Unaccounted: true},
	}
	path := writeArm(t, dir, "arm.json", results)

	want := reportSolvedPerHour(t, renderBody(results))

	report, err := multiCompareReport([]string{path, path})
	if err != nil {
		t.Fatalf("multiCompareReport: %v", err)
	}
	if got := compareSolvedPerHour(t, report); got != want {
		t.Fatalf("compare Solved/hour = %s, suite report says %s for the same artifact\n"+
			"1600s of known wall (one 30-minute killed run) went missing from the compare denominator:\n%s",
			got, want, report)
	}
}

// Same root cause, second printed number: the per-class table counts every
// run's wall (c.WallMs is accumulated before the Unaccounted continue) while the
// global marginal-utility line divided only accounted wall by the same task
// denominator. One section, two populations: the class row says A cost +870.0s
// per task more, the global line said -30.0s less.
func TestMarginalUtilityGlobalWallMatchesThePerClassRows(t *testing.T) {
	dir := testenv.TempDir(t)
	mk := func(id string, wallMs int64, unaccounted bool) result {
		r := result{task: task{ID: id, Class: "bugfix"}, Passed: true, WallMs: wallMs}
		r.Unaccounted = unaccounted
		return r
	}
	a := []result{
		mk("ok", 60_000, false),
		mk("killed", 1_800_000, true),
	}
	b := []result{
		mk("ok", 60_000, false),
		mk("killed2", 60_000, false),
	}
	pathA := writeArm(t, dir, "a.json", a)
	pathB := writeArm(t, dir, "b.json", b)

	report, err := compareReports(pathA, pathB)
	if err != nil {
		t.Fatalf("compareReports: %v", err)
	}
	var row string
	for line := range strings.SplitSeq(report, "\n") {
		if strings.HasPrefix(line, "| bugfix |") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("no bugfix class row:\n%s", report)
	}
	global := regexp.MustCompile(`wall/task ([+-][\d.]+)s`).FindStringSubmatch(report)
	if global == nil {
		t.Fatalf("no global wall/task line:\n%s", report)
	}
	// The one class row reports A 930.0s/task, B 60.0s/task, i.e. +870.0s.
	if global[1] != "+870.0" {
		t.Fatalf("global wall/task = %ss while the per-class row beside it says +870.0s "+
			"(A 930.0s vs B 60.0s): the global figure drops the unaccounted run's wall, "+
			"the class row keeps it\n%s\n%s", global[1], row, report)
	}
}

// compareOutput pins a compare rendering byte for byte. RX_UPDATE_GOLDEN
// rewrites it; the no-unaccounted fixture is the population where the two wall
// totals coincide, so the readout must not move when only the accounting does.
func compareOutput(t *testing.T, dir, name, got string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if os.Getenv("RX_UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("%s: compare output changed\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// A fixture with no unaccounted runs is where every wall is accounted wall:
// both rates must render exactly as they did before the population split.
func TestCompareOutputUnchangedWithoutUnaccountedRuns(t *testing.T) {
	dir := testenv.TempDir(t)
	goldenDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("testdata dir: %v", err)
	}
	t.Chdir(dir)
	a := []result{
		{task: task{ID: "a", Class: "bugfix"}, Passed: true, WallMs: 60_000, Attempt: 1, TTCSMs: 60_000,
			runMetrics: runMetrics{Steps: 7, ToolCalls: 10, PromptTokens: 900, CompletionTokens: 100, Cost: 0.02, Currency: "$"}},
		{task: task{ID: "b", Class: "perf"}, WallMs: 90_000, Attempt: 1,
			runMetrics: runMetrics{Steps: 9, ToolCalls: 14, PromptTokens: 1200, CompletionTokens: 200, Cost: 0.03, Currency: "$"}},
	}
	b := []result{
		{task: task{ID: "a", Class: "bugfix"}, Passed: true, WallMs: 45_000, Attempt: 1, TTCSMs: 45_000,
			runMetrics: runMetrics{Steps: 5, ToolCalls: 10, PromptTokens: 700, CompletionTokens: 100, Cost: 0.01, Currency: "$"}},
		{task: task{ID: "b", Class: "perf"}, WallMs: 90_000, Attempt: 1,
			runMetrics: runMetrics{Steps: 9, ToolCalls: 14, PromptTokens: 1200, CompletionTokens: 200, Cost: 0.03, Currency: "$"}},
	}
	writeArm(t, ".", "a.json", a)
	writeArm(t, ".", "b.json", b)

	ab, err := compareReports("a.json", "b.json")
	if err != nil {
		t.Fatalf("compareReports: %v", err)
	}
	compareOutput(t, goldenDir, "no_unaccounted_ab.golden", ab)

	multi, err := multiCompareReport([]string{"a.json", "b.json"})
	if err != nil {
		t.Fatalf("multiCompareReport: %v", err)
	}
	compareOutput(t, goldenDir, "no_unaccounted_multi.golden", multi)
}

// Wall seconds / solved divides the accounted wall by accounted solves: the
// unaccounted run's wall must stay out of that ratio even though it now enters
// every rate figure.
func TestWallSecondsPerSolvedKeepsAccountedPopulation(t *testing.T) {
	dir := testenv.TempDir(t)
	a := []result{
		{task: task{ID: "ok"}, Passed: true, WallMs: 60_000},
		{task: task{ID: "killed"}, Passed: true, WallMs: 1_800_000, Unaccounted: true},
	}
	b := []result{
		{task: task{ID: "ok"}, Passed: true, WallMs: 60_000},
		{task: task{ID: "ok2"}, Passed: true, WallMs: 60_000},
	}
	report, err := compareReports(writeArm(t, dir, "a.json", a), writeArm(t, dir, "b.json", b))
	if err != nil {
		t.Fatalf("compareReports: %v", err)
	}
	// A: 60s accounted wall over 1 accounted solve; its all-run wall (1860s)
	// would print 1860.0 here, or 930.0 over all solves. B has no unaccounted
	// run, so both walls coincide at 60.0.
	if !strings.Contains(report, "| Wall seconds / solved | 60.0 | 60.0 |") {
		t.Fatalf("Wall seconds / solved is not accounted wall / accounted solves:\n%s", report)
	}
	for _, wrong := range []string{"| Wall seconds / solved | 1860.0 |", "| Wall seconds / solved | 930.0 |"} {
		if strings.Contains(report, wrong) {
			t.Fatalf("Wall seconds / solved divided the all-run wall (%s):\n%s", wrong, report)
		}
	}
}

// The suite report's Solved/hour convention is the reference the compare mode
// now follows: every attempt's wall, solved runs over that total.
func TestSuiteReportSolvedPerHourCountsEveryRun(t *testing.T) {
	withKilled := []result{
		{task: task{ID: "ok"}, Passed: true, WallMs: 60_000},
		{task: task{ID: "killed"}, Passed: true, WallMs: 1_800_000, Unaccounted: true},
	}
	if got := reportSolvedPerHour(t, renderBody(withKilled)); got != "3.9" {
		t.Errorf("suite report Solved/hour = %s, want 3.9 (2 solves over 1860s)", got)
	}
	allAccounted := []result{
		{task: task{ID: "ok"}, Passed: true, WallMs: 60_000},
		{task: task{ID: "ok2"}, Passed: true, WallMs: 60_000},
	}
	if got := reportSolvedPerHour(t, renderBody(allAccounted)); got != "60.0" {
		t.Errorf("suite report Solved/hour = %s, want 60.0 (2 solves over 120s)", got)
	}
}
