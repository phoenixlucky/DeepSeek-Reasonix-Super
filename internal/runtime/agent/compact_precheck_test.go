package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/contract/provider"
)

// When the messages a fold leaves in place already reach the trigger, no summary
// can bring the candidate under it. The refusal comes before the summary is
// paid for, and says what holds the size.
func TestAFoldThatCannotFitIsRefusedBeforeItsSummaryIsPaidFor(t *testing.T) {
	fake := &fakeProvider{reply: "digest"}
	a := windowedFoldFixtureWith(t, 120_000, fake)
	msgs := a.sess.conversation.Snapshot()
	huge := strings.Repeat("y", 600*1024)
	msgs[len(msgs)-1] = provider.Message{Role: provider.RoleUser, Content: huge}
	msgs[len(msgs)-2] = provider.Message{Role: provider.RoleAssistant, Content: huge}
	a.sess.conversation.Replace(msgs)

	_, reason, err := a.window().compactToProjection(context.Background(), CompactionTriggerPressure, "", compactionScope{}, false)
	if !errors.Is(err, errCheckpointRejected) || reason != NoopCandidateAboveTrigger {
		t.Fatalf("verdict = %q, err = %v; want the above-trigger refusal", reason, err)
	}
	if len(fake.got) != 0 {
		t.Fatalf("a summary request was sent for a fold that could not fit (%d messages)", len(fake.got))
	}
	for _, want := range []string{"empty summary", "recent tail", "fixed prefix"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}

// A fold that can fit is not refused: the check only ever sees the parts a
// summary cannot shrink.
func TestAFoldThatCanFitStillRuns(t *testing.T) {
	a := windowedFoldFixture(t, 120_000)
	outcome, reason, err := a.window().compactToProjection(context.Background(), CompactionTriggerPressure, "", compactionScope{}, false)
	if err != nil || outcome != CompactionInstalled {
		t.Fatalf("outcome = %v, reason %q, err %v; want the fold installed", outcome, reason, err)
	}
}
