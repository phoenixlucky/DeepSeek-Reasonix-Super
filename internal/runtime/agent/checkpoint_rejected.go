package agent

import (
	"errors"
	"fmt"
)

// checkpointRejection carries why a fold was judged not worth installing. The
// reason used to live only in the sentence, and the reader cut it back out at
// the sentinel's own wording — so rewording the sentinel silently returned the
// whole error where a reason was expected.
type checkpointRejection struct {
	code   CompactionNoopReason
	reason string
	holds  *candidateHolds
}

func (c *checkpointRejection) Error() string {
	if c.holds == nil {
		return errCheckpointRejected.Error() + ": " + c.reason
	}
	return errCheckpointRejected.Error() + ": " + c.reason + " (" + c.holds.String() + ")"
}

// candidateHolds splits a rejected candidate's size by what it is made of, so a
// fold that cannot get under the line says what keeps it there.
type candidateHolds struct {
	Prefix, Kept, Summary, Tail int
}

func (h candidateHolds) String() string {
	return fmt.Sprintf("fixed prefix %d + protected messages %d + summary %d + recent tail %d tokens", h.Prefix, h.Kept, h.Summary, h.Tail)
}

// attribute names what made a size-based rejection; any other refusal is left
// as it was.
func attribute(err error, h candidateHolds) error {
	var rej *checkpointRejection
	if !errors.As(err, &rej) {
		return err
	}
	switch rej.code {
	case NoopCandidateAboveTrigger, NoopCandidateAboveCeiling, NoopCandidateAbovePhysical:
		rej.holds = &h
	}
	return err
}

func (c *checkpointRejection) Unwrap() error { return errCheckpointRejected }

// rejectCheckpoint builds the verdict with the rule that refused it and the
// figures behind that rule.
func rejectCheckpoint(code CompactionNoopReason, format string, args ...any) error {
	return &checkpointRejection{code: code, reason: fmt.Sprintf(format, args...)}
}
