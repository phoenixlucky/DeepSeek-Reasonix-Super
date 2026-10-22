package control

import (
	"fmt"
	"sync"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

// awaitDelayOverride lets a test stand in for the user's setting, whose floor is ten seconds.
var awaitDelayOverride func() (time.Duration, bool)

// awaitResumeRecheck is how soon a fire that finds the turn still running looks again.
const awaitResumeRecheck = 500 * time.Millisecond

const awaitResumeSource = "host:await-user-timeout"

// awaitResume holds the timer behind a turn that handed its list to the user.
// It exists only while the user's ask-timeout setting is on.
type awaitResume struct {
	mu    sync.Mutex
	timer *time.Timer
	seq   int
	// runs counts host continuations since the user last spoke.
	runs int
}

// awaitResumeLimit bounds how many times in a row the host continues a parked
// list on its own: a model that hands it back every time would otherwise spin
// as long as the user stays away.
const awaitResumeLimit = 3

// userSpoke clears the count when a turn is the user's own words. A host turn,
// including our own continuation, leaves it standing.
func (a *awaitResume) userSpoke(synthetic bool) {
	if synthetic {
		return
	}
	a.mu.Lock()
	a.runs = 0
	a.mu.Unlock()
}

func (a *awaitResume) cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

// armAwaitResume starts the wait after a turn recorded a user gate. Nothing is
// armed when the setting is off or unusable, so the list waits as long as it takes.
func (c *Controller) armAwaitResume(delay time.Duration, ok bool) {
	c.awaitResume.cancel()
	if !ok || delay <= 0 || c.turnUserGate() == "" {
		return
	}
	c.awaitResume.mu.Lock()
	defer c.awaitResume.mu.Unlock()
	c.awaitResume.timer = time.AfterFunc(delay, c.resumeUnansweredGate)
}

// userGateSkipDelay is the user's ask-timeout, read quietly: ask already says
// once when the value cannot be used.
func userGateSkipDelay() (time.Duration, bool) {
	if awaitDelayOverride != nil {
		return awaitDelayOverride()
	}
	d, err := config.LoadForEdit(config.UserConfigPath()).AskTimeoutChecked()
	return d, err == nil
}

// resumeUnansweredGate continues the parked list on the host's word. The model
// is told nobody answered and that its assumptions belong in the record; the
// user's next message still lands as an ordinary turn.
func (c *Controller) resumeUnansweredGate() {
	need := c.turnUserGate()
	if need == "" {
		return
	}
	// Asked before the lock: Running takes c.mu, and nothing may hold c.mu while
	// it waits for this one.
	if c.Running() {
		// The turn that armed this is still unwinding; look again shortly.
		c.awaitResume.mu.Lock()
		c.awaitResume.timer = time.AfterFunc(awaitResumeRecheck, c.resumeUnansweredGate)
		c.awaitResume.mu.Unlock()
		return
	}
	c.awaitResume.mu.Lock()
	c.awaitResume.timer = nil
	if c.awaitResume.runs >= awaitResumeLimit {
		c.awaitResume.mu.Unlock()
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeAwaitGaveUp,
			Text: fmt.Sprintf(i18n.M.AwaitGaveUpFmt, awaitResumeLimit), Detail: need})
		return
	}
	c.awaitResume.runs++
	c.awaitResume.seq++
	seq := c.awaitResume.seq
	c.awaitResume.mu.Unlock()
	c.liftUserGatedGoal(false)
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeAwaitTimedOut,
		Text: fmt.Sprintf(i18n.M.AwaitTimedOutFmt, need), Detail: need})
	_, _ = c.EnqueueHostContinuation(InboxRequest{
		Display:     fmt.Sprintf(i18n.M.AwaitTimedOutFmt, need),
		Submit:      awaitTimedOutPrompt(need),
		Source:      awaitResumeSource,
		Idempotency: fmt.Sprintf("await-timeout-%d-%d", time.Now().UnixNano(), seq),
	})
}

func awaitTimedOutPrompt(need string) string {
	return "You handed this task list to the user and waited for: " + need + "\n" +
		"Nobody responded within the time the user allows, and the user is not at the keyboard. " +
		"Continue the list on your own judgement. Open your reply with a line `Assumptions:` that states each assumption you made in place of their answer, " +
		"so they can correct it when they return. Prefer the choice that is easiest to undo, and do not do anything irreversible that rests on a guessed answer."
}
