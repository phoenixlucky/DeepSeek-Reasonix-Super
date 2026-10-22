package evidence

import (
	"slices"
	"testing"
)

// Collection is idempotent within a turn: the same job is leased once, and a
// job is identified by session and id together.
func TestBackgroundLeaseNotesEachJobOncePerTurn(t *testing.T) {
	l := NewLedger()
	if l.BackgroundLeases() != nil {
		t.Fatal("a fresh ledger holds leases")
	}
	if !l.NoteBackgroundLease("s1", "job-1") {
		t.Fatal("the first lease of a job was refused")
	}
	if l.NoteBackgroundLease("s1", "job-1") {
		t.Fatal("the same job was leased twice in one turn")
	}
	if !l.NoteBackgroundLease("s2", "job-1") || !l.NoteBackgroundLease("s1", "job-2") {
		t.Fatal("a job in another session, or another job in the same one, was treated as a duplicate")
	}
	want := []BackgroundLease{{"s1", "job-1"}, {"s2", "job-1"}, {"s1", "job-2"}}
	if got := l.BackgroundLeases(); !slices.Equal(got, want) {
		t.Fatalf("leases = %v, want %v", got, want)
	}
}

// The slice handed out is a copy: a caller committing leases cannot rewrite
// what the ledger still holds.
func TestBackgroundLeasesAreACopyAndResetStartsANewRun(t *testing.T) {
	l := NewLedger()
	l.NoteBackgroundLease("s", "a")
	got := l.BackgroundLeases()
	got[0].JobID = "tampered"
	if l.BackgroundLeases()[0].JobID != "a" {
		t.Fatal("the caller's slice aliases the ledger's")
	}
	l.Record(Receipt{ToolName: "read_file", Success: true, Paths: []string{"x.go"}})
	l.ResetBackgroundLeases()
	if l.BackgroundLeases() != nil || !l.NoteBackgroundLease("s", "a") {
		t.Fatal("a reset did not make the job collectable again")
	}
	if len(l.Summary().Receipts) != 1 {
		t.Fatal("a reset dropped the durable receipts")
	}
	var nilLedger *Ledger
	nilLedger.ResetBackgroundLeases()
	if nilLedger.NoteBackgroundLease("s", "a") || nilLedger.BackgroundLeases() != nil {
		t.Fatal("a nil ledger leased something")
	}
}
