package evidence

import (
	"encoding/json"
	"testing"
)

func mutationAt(path string) Receipt {
	return Receipt{ToolName: "write_file", Success: true, Write: true, Mutation: true, Paths: []string{path}}
}

func reviewReceipt(kind ReviewKind, verdict ReviewVerdict, grant ...ReviewKind) Receipt {
	args, _ := json.Marshal(map[string]any{"kind": kind, "verdict": verdict, "reviewed_paths": []string{"a.go"}})
	r := Receipt{ToolName: reviewReportTool, Success: true, SourceExecutionID: "exec-1", Args: args}
	if len(grant) > 0 {
		g := GrantReviewAuthority(grant...)
		r.ReviewAuthority = &g
	}
	return r
}

func baselineOf(t *testing.T, receipts ...Receipt) (int, bool) {
	t.Helper()
	l := NewLedger()
	for _, r := range receipts {
		l.Record(r)
	}
	return l.UnreviewedMutationBaseline()
}

// The change set starts at the first mutation no accepted review has covered,
// and a later harmless write cannot shrink what an earlier one put in scope.
func TestUnreviewedMutationBaselineStartsAtTheFirstUncoveredWrite(t *testing.T) {
	if _, ok := baselineOf(t); ok {
		t.Fatal("an empty ledger has a change set")
	}
	if _, ok := baselineOf(t, Receipt{ToolName: "read_file", Success: true, Paths: []string{"a.go"}}); ok {
		t.Fatal("a read opened a change set")
	}
	if at, ok := baselineOf(t, mutationAt("a.go"), mutationAt("b.go")); !ok || at != 0 {
		t.Fatalf("unreviewed writes: baseline = %d,%v, want 0,true", at, ok)
	}
	both := []ReviewKind{ReviewKindReview, ReviewKindSecurity}
	covered := []Receipt{
		mutationAt("a.go"),
		reviewReceipt(ReviewKindReview, ReviewVerdictPass, both...),
		reviewReceipt(ReviewKindSecurity, ReviewVerdictPass, both...),
	}
	if _, ok := baselineOf(t, covered...); ok {
		t.Fatal("a write covered by both accepted reviews is still in the change set")
	}
	if at, ok := baselineOf(t, append(covered, mutationAt("c.go"))...); !ok || at != 3 {
		t.Fatalf("a write after both reviews: baseline = %d,%v, want 3,true", at, ok)
	}
}

// With only one kind reviewed, the other still owes the earlier write, so the
// baseline is the widest window of the two.
func TestUnreviewedMutationBaselineIsTheWidestPerKindWindow(t *testing.T) {
	receipts := []Receipt{
		mutationAt("a.go"),
		reviewReceipt(ReviewKindReview, ReviewVerdictPass, ReviewKindReview),
		mutationAt("b.go"),
	}
	if at, ok := baselineOf(t, receipts...); !ok || at != 0 {
		t.Fatalf("baseline = %d,%v, want 0,true: security never covered the first write", at, ok)
	}
}

// A report the host did not grant, one with no execution behind it, and one
// that blocks all leave the change set where it was.
func TestUnreviewedMutationBaselineIgnoresReviewsTheHostDidNotAccept(t *testing.T) {
	both := []ReviewKind{ReviewKindReview, ReviewKindSecurity}
	ungranted := reviewReceipt(ReviewKindReview, ReviewVerdictPass)
	unattributed := reviewReceipt(ReviewKindReview, ReviewVerdictPass, both...)
	unattributed.SourceExecutionID = ""
	failed := reviewReceipt(ReviewKindReview, ReviewVerdictPass, both...)
	failed.Success = false
	blocking := reviewReceipt(ReviewKindReview, ReviewVerdictBlock, both...)
	malformed := reviewReceipt(ReviewKindReview, ReviewVerdictPass, both...)
	malformed.Args = json.RawMessage(`{"kind":"review"}`)
	for name, r := range map[string]Receipt{"ungranted": ungranted, "unattributed": unattributed, "failed": failed, "blocking": blocking, "malformed": malformed} {
		if at, ok := baselineOf(t, mutationAt("a.go"), r); !ok || at != 0 {
			t.Errorf("%s review closed the change set: baseline = %d,%v", name, at, ok)
		}
	}
	var nilLedger *Ledger
	if _, ok := nilLedger.UnreviewedMutationBaseline(); ok {
		t.Fatal("a nil ledger has a change set")
	}
}
