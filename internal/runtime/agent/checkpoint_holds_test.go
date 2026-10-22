package agent

import (
	"errors"
	"strings"
	"testing"

	"reasonix/internal/contract/provider"
)

// A fold that cannot get under the trigger says what keeps it there; the rule
// that refused it, and the sentinel callers match, are what they were.
func TestARejectedCandidateNamesWhatHoldsItsSize(t *testing.T) {
	a, _ := failureFixture(t, &fakeProvider{reply: "digest"})
	w := a.window()
	trigger := w.compactTrigger()
	err := w.acceptCheckpointCandidate(CompactionTriggerPressure, compactionScope{}, trigger*4, trigger, 10)
	err = attribute(err, candidateHolds{Prefix: 10, Kept: trigger - 500, Summary: 400, Tail: 90})
	if !errors.Is(err, errCheckpointRejected) || compactionFailureCode(err) != NoopCandidateAboveTrigger {
		t.Fatalf("the rule or the sentinel changed: %v", err)
	}
	for _, want := range []string{"fixed prefix 10", "protected messages", "summary 400", "recent tail 90"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}

func TestARefusalThatIsNotAboutSizeIsLeftAlone(t *testing.T) {
	err := attribute(rejectCheckpoint(NoopCandidateNotSmaller, "candidate would not reduce tokens (5 >= 5)"), candidateHolds{Kept: 1})
	if strings.Contains(err.Error(), "protected messages") {
		t.Fatalf("a not-smaller refusal was given a size breakdown: %v", err)
	}
	if plain := errors.New("summary failed"); !errors.Is(attribute(plain, candidateHolds{}), plain) {
		t.Fatal("an error that is not a rejection was replaced")
	}
}

func TestCandidateHoldsAccountForTheWholeCandidate(t *testing.T) {
	a, _ := failureFixture(t, &fakeProvider{reply: "digest"})
	kept := []provider.Message{{Role: provider.RoleUser, Content: strings.Repeat("kept words ", 200)}}
	const total = 5000
	h := a.window().candidateHolds(total, 300, kept, strings.Repeat("summary ", 100))
	if h.Prefix != 300 || h.Kept == 0 || h.Summary == 0 {
		t.Fatalf("holds = %+v", h)
	}
	if h.Prefix+h.Kept+h.Summary+h.Tail != total {
		t.Fatalf("the parts %+v do not add up to the candidate %d", h, total)
	}
}
