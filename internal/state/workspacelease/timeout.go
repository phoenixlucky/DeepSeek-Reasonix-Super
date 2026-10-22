package workspacelease

import (
	"errors"
	"fmt"
	"time"
)

// CodeClaimTimeout names a wait that ran out its limit, as distinct from one a
// caller cancelled or one refused outright as a conflict.
const CodeClaimTimeout = "workspace.claim_timeout"

var ErrClaimTimeout = errors.New("workspace write claim wait timed out")

// ClaimTimeoutError is a blocked acquisition that stopped waiting because the
// session's maximum wait ran out. Conflict is what it was waiting on.
type ClaimTimeoutError struct {
	Conflict *ConflictError
	Waited   time.Duration
	Max      time.Duration
}

func (e *ClaimTimeoutError) RefusalCode() string  { return CodeClaimTimeout }
func (e *ClaimTimeoutError) Is(target error) bool { return target == ErrClaimTimeout }
func (e *ClaimTimeoutError) Unwrap() error {
	if e.Conflict == nil {
		return nil
	}
	return e.Conflict
}

func (e *ClaimTimeoutError) Error() string {
	held := ""
	if c := e.Conflict; c != nil {
		held = fmt.Sprintf(" session %q (%q) holds write claim %q;", c.Holder, c.SessionID, c.Paths)
	}
	return fmt.Sprintf("waited %s for the workspace write claim and stopped at the limit of %s;%s the write was not made (refusal: %s)", e.Waited.Round(time.Second), e.Max, held, CodeClaimTimeout)
}

// WithMaxWait bounds how long one acquisition blocks behind another session
// before it fails with ClaimTimeoutError. Zero, the default, waits as long as
// the caller's context lasts.
func WithMaxWait(d time.Duration) Option {
	return func(o *Owner) { o.maxWait = max(d, 0) }
}

func (w *waitClock) expired() bool {
	return w.owner.maxWait > 0 && !w.started.IsZero() && time.Since(w.started) >= w.owner.maxWait
}

func (w *waitClock) timeout() error {
	return &ClaimTimeoutError{Conflict: w.conflict, Waited: time.Since(w.started), Max: w.owner.maxWait}
}
