package scheduler

import (
	"context"
	"errors"
	"time"

	"reasonix/internal/runtime/schedrun"
	"reasonix/internal/state/schedule"
)

// DefaultLateGrace is how late a slot may still start. It is never more than a
// quarter of the schedule's interval.
const DefaultLateGrace = 15 * time.Minute

// Runner runs a claimed trigger to its end and settles it. *schedrun.Supervisor
// is the production one.
type Runner interface {
	Run(ctx context.Context, triggerID string) (schedrun.Report, error)
}

// Scheduler is one driver of the store.
type Scheduler struct {
	Store  *schedule.Store
	Policy schedule.Policy
	Runner Runner
	// Now is the clock; nil is the store's own wall clock.
	Now func() time.Time
}

// Skipped is a slot that was deliberately not run, with the typed reason.
type Skipped struct {
	ScheduleID string
	Slot       time.Time
	Reason     schedule.SkipReason
}

// Refused is a slot the store would not admit, by the sentinel it answered with.
type Refused struct {
	ScheduleID string
	Err        error
}

// Result is what one tick did.
type Result struct {
	Reaped  []string
	Skipped []Skipped
	Refused []Refused
	Reports []schedrun.Report
	// Paused says the global switch was on and nothing was claimed.
	Paused bool
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Tick reaps runs whose executor is gone, claims at most one due slot, runs it
// and returns. It never starts a second run while one is in flight: the store
// refuses it, and the slot waits for a later tick or is skipped once past grace.
func (s *Scheduler) Tick(ctx context.Context) (Result, error) {
	var out Result
	reaped, err := s.Store.ReapDead(ctx, s.Policy)
	out.Reaped = reaped
	if err != nil {
		return out, err
	}
	m, _, err := s.Store.Snapshot(ctx)
	if err != nil {
		return out, err
	}
	if m.PausedAll {
		out.Paused = true
		return out, nil
	}
	now := s.now()
	for _, sc := range m.Schedules {
		if sc.Status != schedule.StatusActive {
			continue
		}
		slot, ok := sc.Trigger.LatestSlot(now)
		if !ok || hasRun(m, schedule.TriggerID(sc.ID, slot)) {
			continue
		}
		if now.Sub(slot) > graceFor(sc) {
			s.skip(ctx, &out, sc, slot, schedule.SkipMissed)
			continue
		}
		run, err := s.Store.Claim(ctx, s.Policy, sc.ID, slot)
		switch {
		case err == nil:
			rep, runErr := s.Runner.Run(ctx, run.TriggerID)
			out.Reports = append(out.Reports, rep)
			return out, runErr
		case errors.Is(err, schedule.ErrConcurrency), errors.Is(err, schedule.ErrAlreadyClaimed),
			errors.Is(err, schedule.ErrSlotStale), errors.Is(err, schedule.ErrNotDue):
			// Another run is in flight, or someone else has this slot.
		case errors.Is(err, schedule.ErrBudget):
			s.skip(ctx, &out, sc, slot, schedule.SkipBudgetWindow)
		default:
			out.Refused = append(out.Refused, Refused{ScheduleID: sc.ID, Err: err})
		}
	}
	return out, nil
}

func (s *Scheduler) skip(ctx context.Context, out *Result, sc schedule.Schedule, slot time.Time, why schedule.SkipReason) {
	if err := s.Store.RecordSkip(ctx, sc.ID, slot, why); err == nil {
		out.Skipped = append(out.Skipped, Skipped{ScheduleID: sc.ID, Slot: slot, Reason: why})
	}
}

func hasRun(m schedule.Manifest, triggerID string) bool {
	for _, r := range m.Runs {
		if r.TriggerID == triggerID {
			return true
		}
	}
	return false
}

// graceFor is how late this schedule's slot may start: the default, or a quarter
// of the interval when that is shorter.
func graceFor(sc schedule.Schedule) time.Duration {
	if sc.Trigger.Kind == schedule.TriggerEvery {
		if q := time.Duration(sc.Trigger.EverySec) * time.Second / 4; q < DefaultLateGrace {
			return q
		}
	}
	return DefaultLateGrace
}
