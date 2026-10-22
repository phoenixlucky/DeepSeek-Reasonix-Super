package scheduler

import (
	"errors"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/state/schedule"
)

// OpenUserStore opens the machine-wide schedule store and resolves the policy
// from the user's configuration alone: a repository's file is never read, so it
// cannot move a ceiling. A nil now uses the wall clock.
func OpenUserStore(now func() time.Time) (*schedule.Store, schedule.Policy, error) {
	roots := config.RootsForHome("")
	dir := roots.ScheduleDir()
	if dir == "" {
		return nil, schedule.Policy{}, errors.New("the state directory is unknown")
	}
	store, err := schedule.Open(dir)
	if err != nil {
		return nil, schedule.Policy{}, err
	}
	user, err := schedule.LoadOverrides(roots.UserConfigLoadPath())
	if err != nil {
		return nil, schedule.Policy{}, err
	}
	policy, _, err := schedule.Resolve(user, schedule.Overrides{})
	if err != nil {
		return nil, schedule.Policy{}, err
	}
	if now != nil {
		store = store.WithClock(now)
	}
	return store, policy, nil
}
