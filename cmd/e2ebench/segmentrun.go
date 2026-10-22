package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// runSegments drives a task's legs and folds their accounting into one result.
// Each leg writes its own metrics file: sharing one path would let the last
// leg's numbers stand in for the whole run, and the tokens the earlier legs
// spent would simply vanish. Only the final leg's trajectory digest is kept;
// Segments records how many legs it does not cover.
func runSegments(ctx context.Context, cfg suiteConfig, t task, work, trajDir string, env []string, r *result) error {
	segs := planSegments(t, cfg.segments, cfg.steers)
	r.Segments = len(segs)
	// Out of the work dir, and named without the task id: one run spent a tool
	// call on the harness's own token counts, and another found this directory
	// by grepping the temp root for its task — moving it only helps unnamed.
	metricsDir, err := os.MkdirTemp("", "e2ebench-metrics-")
	// A snapshot recovered from the sidecar is the only surviving evidence of
	// what that leg spent, so its directory is kept instead of removed.
	keepMetrics := false
	if err != nil {
		metricsDir = work
	} else {
		defer func() {
			if !keepMetrics {
				_ = os.RemoveAll(metricsDir)
			}
		}()
	}
	var runErr error
	for _, seg := range segs {
		metricsPath := filepath.Join(metricsDir, fmt.Sprintf(".run-metrics-%d.json", seg.index))
		trajPath := segmentTrajectoryPath(trajDir, trajectoryStem(t.ID, cfg.trial, cfg.trials), seg, len(segs))
		args := buildSegmentArgs(cfg, seg, metricsPath, trajPath)

		cmd := exec.CommandContext(ctx, cfg.bin, args...)
		cmd.Dir = work
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		cmd.WaitDelay = 10 * time.Second
		runErr = cmd.Run()

		m, partial, err := legMetrics(metricsPath)
		if err != nil {
			// The leg left no record at all: nothing survives to say what it
			// spent, so the run is unmeasured rather than priced at zero.
			r.Unaccounted = true
		} else {
			// A snapshot is real evidence, so the run stays accounted and only
			// Partial says its numbers stop at the last write. It is sticky: a
			// complete later leg cannot restore the tail an earlier one lost.
			r.Partial = r.Partial || partial || !m.Complete
			keepMetrics = keepMetrics || partial
			foldSegmentMetrics(r, m, seg.index)
		}
		// A leg that died takes the run with it: resuming a session the child
		// never finished writing would measure the harness's crash recovery,
		// which is a different experiment.
		if runErr != nil || ctx.Err() != nil {
			break
		}
	}
	return runErr
}

// legMetrics reads one leg's accounting. The final record is authoritative and
// the in-flight snapshot is only read when the final one is absent, so the two
// can never be counted together.
func legMetrics(path string) (m runMetrics, partial bool, err error) {
	if m, err = readMetrics(path); err == nil {
		return m, false, nil
	}
	if m, err = readMetrics(path + ".partial"); err == nil {
		return m, true, nil
	}
	return m, false, err
}

// trajectoryStem names one run's trajectory files: the task id, and the trial
// when -trials repeats the task, so a later trial cannot overwrite an earlier
// one. A single run keeps the bare id every reader already knows.
func trajectoryStem(id string, trial, trials int) string {
	if trials < 2 {
		return id
	}
	return fmt.Sprintf("%s.trial-%d", id, trial)
}

// trajectoryTaskID is the task a trajectory file recorded: its name without
// the trial mark of trajectoryStem or the leg mark of segmentTrajectoryPath.
func trajectoryTaskID(path string) string {
	id := strings.TrimSuffix(filepath.Base(path), ".trajectory.jsonl")
	id = trimNumberedMark(id, ".seg")
	return trimNumberedMark(id, ".trial-")
}

func trimNumberedMark(id, mark string) string {
	i := strings.LastIndex(id, mark)
	if i < 0 {
		return id
	}
	if _, err := strconv.Atoi(id[i+len(mark):]); err != nil {
		return id
	}
	return id[:i]
}

// segmentTrajectoryPath keeps one file per leg so a resumed leg cannot truncate
// the record of the one before it.
func segmentTrajectoryPath(dir, id string, seg segment, total int) string {
	if dir == "" {
		return ""
	}
	if total < 2 {
		return filepath.Join(dir, id+".trajectory.jsonl")
	}
	return filepath.Join(dir, fmt.Sprintf("%s.seg%d.trajectory.jsonl", id, seg.index))
}

// lastSegmentTrajectory names the file whose digest represents the run. Only
// the final leg's is read; Segments in the JSON says how many were not.
func lastSegmentTrajectory(dir, id string, segments int) string {
	if dir == "" {
		return ""
	}
	if segments < 2 {
		return filepath.Join(dir, id+".trajectory.jsonl")
	}
	return filepath.Join(dir, fmt.Sprintf("%s.seg%d.trajectory.jsonl", id, segments))
}

// foldSegmentMetrics adds one leg's spend to the run. The first leg's metrics
// establish the non-additive fields (currency, outcome); later legs contribute
// their totals, and the last leg's outcome wins because it is the one that
// ended the run.
func foldSegmentMetrics(r *result, m runMetrics, index int) {
	if index == 1 {
		r.runMetrics = m
		return
	}
	r.PromptTokens += m.PromptTokens
	r.CompletionTokens += m.CompletionTokens
	r.CacheHitTokens += m.CacheHitTokens
	r.CacheMissTokens += m.CacheMissTokens
	r.Cost += m.Cost
	r.Steps += m.Steps
	r.Compactions += m.Compactions
	r.ToolCalls += m.ToolCalls
	r.ToolFailures += m.ToolFailures
	r.SubagentToolCalls += m.SubagentToolCalls
	r.Retries += m.Retries
	r.Complete = m.Complete
	if m.Outcome != "" {
		r.Outcome = m.Outcome
	}
	for name, n := range m.ToolCallsByName {
		if r.ToolCallsByName == nil {
			r.ToolCallsByName = map[string]int{}
		}
		r.ToolCallsByName[name] += n
	}
	for reason, n := range m.PrefixChangeReasonCounts {
		if r.PrefixChangeReasonCounts == nil {
			r.PrefixChangeReasonCounts = map[string]int{}
		}
		r.PrefixChangeReasonCounts[reason] += n
	}
	accumulateSources(r.UsageBySource, m.UsageBySource)
}
