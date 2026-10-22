package control

import (
	"fmt"
	"sync"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

const (
	goalRetrySource  = "host:goal-retry"
	goalRetryBase    = 30 * time.Second
	goalRetryRecheck = 500 * time.Millisecond
)

// goalRetry holds the retry behind a Goal paused because its completion check
// could not run. It exists only while the user's goal_auto_retry is above zero.
type goalRetry struct {
	mu    sync.Mutex
	timer *time.Timer
	runs  int
}

// goalRetryDelay is the wait before attempt n (1-based): 30 s, 60 s, 120 s...
var goalRetryDelay = func(n int) time.Duration { return goalRetryBase << (n - 1) }

func (r *goalRetry) cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}

// userSpoke drops the timer and the count when the user's own words start a
// turn; a host turn, including our own retry, leaves the count standing.
func (r *goalRetry) userSpoke(synthetic bool) {
	if synthetic {
		return
	}
	r.cancel()
	r.mu.Lock()
	r.runs = 0
	r.mu.Unlock()
}

// afterGoalTurn runs once a Goal turn has been judged. A pause on the
// evaluator arms the next try while the allowance lasts; any other outcome
// ends the streak, because the check ran.
func (c *Controller) afterGoalTurn() {
	if !c.goals.pausedOnEvaluator() {
		c.goalRetry.cancel()
		c.goalRetry.mu.Lock()
		c.goalRetry.runs = 0
		c.goalRetry.mu.Unlock()
		return
	}
	limit := config.LoadForEdit(config.UserConfigPath()).GoalAutoRetryLimit()
	// With no evaluator configured a retry cannot change the verdict.
	if limit == 0 || c.evaluator == nil {
		return
	}
	r := &c.goalRetry
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs >= limit {
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeGoalRetryGaveUp,
			Text: fmt.Sprintf(i18n.M.GoalRetryGaveUpFmt, limit)})
		return
	}
	r.runs++
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(goalRetryDelay(r.runs), c.retryPausedGoal)
}

func (c *Controller) retryPausedGoal() {
	if !c.goals.pausedOnEvaluator() {
		return
	}
	if c.Running() {
		// The turn that armed this is still unwinding; look again shortly.
		c.goalRetry.mu.Lock()
		c.goalRetry.timer = time.AfterFunc(goalRetryRecheck, c.retryPausedGoal)
		c.goalRetry.mu.Unlock()
		return
	}
	c.goalRetry.mu.Lock()
	n := c.goalRetry.runs
	c.goalRetry.timer = nil
	c.goalRetry.mu.Unlock()
	if !c.ResumeGoal() {
		return
	}
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeGoalRetry,
		Text: fmt.Sprintf(i18n.M.GoalRetryFmt, n)})
	_, _ = c.EnqueueHostContinuation(InboxRequest{
		Display: fmt.Sprintf(i18n.M.GoalRetryFmt, n),
		Submit: "The check that decides whether the Goal is complete could not run after your last turn, so the host paused it and is now retrying. " +
			"Carry on with the Goal; if the work is already finished, say so with update_goal.",
		Source:      goalRetrySource,
		Idempotency: fmt.Sprintf("goal-retry-%d-%d", time.Now().UnixNano(), n),
	})
}

// pausedOnEvaluator reports a Goal waiting because its completion check failed.
func (g *goalMachine) pausedOnEvaluator() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status == GoalStatusBlocked && g.stopCause == stopCauseEvaluator
}
