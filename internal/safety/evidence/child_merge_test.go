package evidence

import (
	"slices"
	"testing"
)

func childWrite(tool string, ok bool, paths ...string) Receipt {
	return Receipt{ToolName: tool, Success: ok, Write: true, Mutation: true, Paths: paths}
}

// A failed child receipt is kept for audit but is never a mutation: only a
// successful state change counts toward the parent's delivery gates.
func TestChildSummaryCountsOnlySuccessfulMutations(t *testing.T) {
	none := ChildEvidenceSummary{}
	if none.HasMutation() || len(none.MutationPaths()) != 0 {
		t.Fatal("an empty summary reported a mutation")
	}
	failed := ChildEvidenceSummary{Receipts: []Receipt{childWrite("write_file", false, "a.go")}}
	if failed.HasMutation() || len(failed.MutationPaths()) != 0 {
		t.Fatal("a failed write counted as a mutation")
	}
	read := ChildEvidenceSummary{Receipts: []Receipt{{ToolName: "read_file", Success: true, Paths: []string{"a.go"}}}}
	if read.HasMutation() {
		t.Fatal("a read counted as a mutation")
	}
	wrote := ChildEvidenceSummary{Receipts: []Receipt{childWrite("write_file", true, "b.go", "a.go", "b.go", "")}}
	if !wrote.HasMutation() {
		t.Fatal("a successful write did not count")
	}
	if got := wrote.MutationPaths(); !slices.Equal(got, []string{"a.go", "b.go"}) {
		t.Fatalf("MutationPaths = %v, want sorted and distinct without blanks", got)
	}
}

// What the child looked at is evidence too, and a failed attempt is not.
func TestChildEvidencePathsIncludeReadsButNotFailures(t *testing.T) {
	s := ChildEvidenceSummary{Receipts: []Receipt{
		{ToolName: "read_file", Success: true, Paths: []string{"z.go", "a.go"}},
		childWrite("write_file", true, "a.go"),
		{ToolName: "read_file", Success: false, Paths: []string{"never.go"}},
	}}
	if got := s.EvidencePaths(); !slices.Equal(got, []string{"a.go", "z.go"}) {
		t.Fatalf("EvidencePaths = %v", got)
	}
}

// The parent already owns its own bookkeeping, so a child's todo and question
// receipts are not merged, and a failed child receipt keeps Success=false.
func TestMergeChildDropsBookkeepingAndKeepsFailuresFailed(t *testing.T) {
	parent := NewLedger()
	parent.MergeChildren(
		ChildEvidenceSummary{Receipts: []Receipt{
			{ToolName: "todo_write", Success: true},
			{ToolName: "complete_step", Success: true},
			{ToolName: "complete_subtask", Success: true},
			{ToolName: "ask", Success: true},
			childWrite("write_file", true, "a.go"),
		}},
		ChildEvidenceSummary{Receipts: []Receipt{childWrite("edit_file", false, "b.go")}},
	)
	got := parent.Summary().Receipts
	if len(got) != 2 {
		t.Fatalf("merged %d receipts, want the two real tool calls: %+v", len(got), got)
	}
	if got[0].ToolName != "write_file" || !got[0].Success || got[1].ToolName != "edit_file" || got[1].Success {
		t.Fatalf("merged receipts lost order or success: %+v", got)
	}
	if parent.Summary().HasMutation() != true || !slices.Equal(parent.Summary().MutationPaths(), []string{"a.go"}) {
		t.Fatalf("the failed child write leaked into the parent's mutations: %v", parent.Summary().MutationPaths())
	}
	var nilLedger *Ledger
	nilLedger.MergeChildren(ChildEvidenceSummary{Receipts: []Receipt{childWrite("write_file", true, "a.go")}})
	NewLedger().MergeChild(ChildEvidenceSummary{})
}
