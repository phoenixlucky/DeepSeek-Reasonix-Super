package control

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/runtime/goaleval"
	"reasonix/internal/state/sessionstore"
)

// stepEvaluator fails the first `failures` calls, then says complete.
type stepEvaluator struct {
	mu       sync.Mutex
	failures int
	calls    int
}

func (s *stepEvaluator) Evaluate(context.Context, goaleval.GoalEvidence) (goaleval.Verdict, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls <= s.failures {
		return goaleval.Verdict{}, errors.New("evaluator offline")
	}
	return goaleval.Verdict{Outcome: goaleval.OutcomeComplete, Reason: "done"}, nil
}

func (s *stepEvaluator) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func goalRetryController(t *testing.T, retries int, eval goaleval.Evaluator) (*Controller, func() []event.Event) {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	cfg := config.LoadForEdit(config.UserConfigPath())
	cfg.Agent.GoalAutoRetry = retries
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	prev := goalRetryDelay
	goalRetryDelay = func(int) time.Duration { return 20 * time.Millisecond }
	t.Cleanup(func() { goalRetryDelay = prev })
	prov := &scriptedTurns{turns: [][]provider.Chunk{textTurn("done.")}}
	ag := agent.New(prov, goalRegistry(), sessionstore.NewSession(""), agent.Options{}, event.Discard)
	var mu sync.Mutex
	var notices []event.Event
	dir := testenv.TempDir(t)
	c := New(Options{
		Runner: ag, Executor: ag, GoalEvaluator: eval,
		SessionDir: dir, SessionPath: filepath.Join(dir, "session.jsonl"),
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice {
				mu.Lock()
				notices = append(notices, e)
				mu.Unlock()
			}
		}),
	})
	t.Cleanup(func() { c.Close(); c.autosaveWG.Wait() })
	return c, func() []event.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]event.Event(nil), notices...)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func hasCode(list []event.Event, code string) bool {
	for _, n := range list {
		if n.Code == code {
			return true
		}
	}
	return false
}

// A check that fails twice and then works finishes the Goal with no click.
func TestGoalRetriesAFailedCheckWithinTheAllowance(t *testing.T) {
	eval := &stepEvaluator{failures: 2}
	c, notices := goalRetryController(t, 2, eval)
	c.Submit("/goal evaluate this")
	waitFor(t, "the Goal to complete", func() bool { return c.GoalStatus() == GoalStatusComplete })
	if eval.count() != 3 {
		t.Fatalf("evaluator calls = %d, want 3 (one try and two retries)", eval.count())
	}
	if !hasCode(notices(), event.NoticeCodeGoalRetry) {
		t.Fatalf("no goal_retry notice among %+v", notices())
	}
}

// A check that never works is retried exactly the allowance, then left to the user.
func TestGoalRetryStopsAtTheAllowance(t *testing.T) {
	eval := &stepEvaluator{failures: 100}
	c, notices := goalRetryController(t, 2, eval)
	c.Submit("/goal evaluate this")
	waitFor(t, "the give-up notice", func() bool { return hasCode(notices(), event.NoticeCodeGoalRetryGaveUp) })
	if eval.count() != 3 {
		t.Fatalf("evaluator calls = %d, want 3", eval.count())
	}
	if rt := c.GoalRuntime(); rt.StopCause != stopCauseEvaluator {
		t.Fatalf("stop cause = %q, want the Goal left paused on the evaluator", rt.StopCause)
	}
}

// Off, the default, leaves the pause to the user.
func TestGoalIsNotRetriedByDefault(t *testing.T) {
	eval := &stepEvaluator{failures: 100}
	c, notices := goalRetryController(t, 0, eval)
	c.Submit("/goal evaluate this")
	waitFor(t, "the pause", func() bool { return c.GoalRuntime().StopCause == stopCauseEvaluator })
	time.Sleep(200 * time.Millisecond)
	if eval.count() != 1 || hasCode(notices(), event.NoticeCodeGoalRetry) {
		t.Fatalf("evaluator calls = %d with retries off, want 1", eval.count())
	}
}

func TestGoalAutoRetryIsClamped(t *testing.T) {
	cfg := &config.Config{}
	for in, want := range map[int]int{-3: 0, 0: 0, 2: 2, 99: config.MaxGoalAutoRetry} {
		cfg.Agent.GoalAutoRetry = in
		if got := cfg.GoalAutoRetryLimit(); got != want {
			t.Errorf("goal_auto_retry %d -> %d, want %d", in, got, want)
		}
	}
}

// With no evaluator configured a retry could never change the verdict, so none is made.
func TestGoalWithoutAnEvaluatorIsNotRetried(t *testing.T) {
	c, notices := goalRetryController(t, 3, nil)
	c.Submit("/goal evaluate this")
	waitFor(t, "the pause", func() bool { return c.GoalRuntime().StopCause == stopCauseEvaluator })
	time.Sleep(200 * time.Millisecond)
	if hasCode(notices(), event.NoticeCodeGoalRetry) {
		t.Fatal("a Goal with no evaluator was retried")
	}
}
