package evidence

import (
	"slices"
	"strings"
	"testing"
)

func TestMakingAnUnprovenChangeAgainDoesNotSettleIt(t *testing.T) {
	got := owed(ledgerOf(opaqueWrite, observedWrite))
	if !slices.Contains(got, ObligationUnprovenMutation) {
		t.Fatalf("obligations = %v, want the unproven mutation to stand: a later write the host could read says nothing of what the first one touched", got)
	}
}

func TestUnprovenMutationPromisesNoRemedyTheModelCanRun(t *testing.T) {
	for _, o := range ledgerOf(opaqueWrite).Obligations(CheckContract{}) {
		if o.Kind != ObligationUnprovenMutation {
			continue
		}
		if !strings.Contains(o.Discharge, "conclude_blocked") {
			t.Fatalf("discharge = %q, want it to name conclude_blocked", o.Discharge)
		}
		for _, promise := range []string{"again", "redo", "re-do", "reports its paths"} {
			if strings.Contains(strings.ToLower(o.Discharge), promise) && !strings.Contains(o.Discharge, "does not") {
				t.Fatalf("discharge = %q, promises a remedy (%q)", o.Discharge, promise)
			}
		}
		return
	}
	t.Fatal("no unproven mutation obligation")
}
