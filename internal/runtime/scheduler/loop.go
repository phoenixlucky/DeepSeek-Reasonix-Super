package scheduler

import (
	"context"
	"time"
)

// DefaultEvery is how often a loop considers a tick.
const DefaultEvery = time.Minute

// Loop ticks on an interval, but only while it is allowed to and while it holds
// the machine's one ticker lock. It never decides what runs: Tick does.
type Loop struct {
	// Tick runs one tick; it builds what it needs afresh so a changed policy
	// applies at the next one.
	Tick func(ctx context.Context) (Result, error)
	// Eligible says this process may spend on the user's behalf now: it has a
	// supervised lifetime and a surface the person can see.
	Eligible func() bool
	// Lock takes the machine-wide ticker lock, failing when another process has it.
	Lock func() (release func(), err error)
	// Every is the interval; zero is DefaultEvery.
	Every time.Duration
	// Done sees what each tick did.
	Done func(Result, error)
}

// Run blocks until ctx ends.
func (l *Loop) Run(ctx context.Context) {
	every := l.Every
	if every <= 0 {
		every = DefaultEvery
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.once(ctx)
		}
	}
}

func (l *Loop) once(ctx context.Context) {
	if !l.Eligible() {
		return
	}
	release, err := l.Lock()
	if err != nil {
		return
	}
	defer release()
	res, err := l.Tick(ctx)
	if l.Done != nil {
		l.Done(res, err)
	}
}
