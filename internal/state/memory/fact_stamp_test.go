package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// StampOfFact and SameContent answer from the one file a fact lives at. The
// package's directory seam records every listing, so a name that was resolved by
// scanning the store would show up here — and it is armed to fail, so nothing on
// this path may consult a directory at all.
func TestFactStampReadsOnlyTheFactsOwnFile(t *testing.T) {
	dir := testenv.TempDir(t)
	s := Store{Dir: filepath.Join(dir, "memory")}
	path, err := s.Save(Memory{
		Name: "deploy-target", Description: "where the staging deploy goes",
		Type: TypeProject, Body: "old body marker",
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, ok, err := s.Read("deploy-target")
	if err != nil || !ok {
		t.Fatalf("Read: ok=%v err=%v", ok, err)
	}

	var listed []string
	withFault(t, func(op, at string) error {
		if op == "read-dir" {
			listed = append(listed, at)
			return os.ErrPermission
		}
		return nil
	})

	stamp, ok := s.StampOfFact(fact)
	if !ok {
		t.Fatal("StampOfFact found no file at the fact's own path")
	}
	// FactAt is the stamp's parsing counterpart: it answers the same one file,
	// and it is what tells a caller whether those bytes still describe the fact
	// it was handed.
	if at, ok := FactAt(stamp.Path); !ok || at.Body != "old body marker" || at.ID != fact.ID {
		t.Fatalf("FactAt did not re-parse the fact's own file: ok=%v id=%q body=%q", ok, at.ID, at.Body)
	}
	if !stamp.SameContent() {
		t.Fatal("SameContent called an untouched fact changed")
	}
	if len(listed) != 0 {
		t.Fatalf("stamping the tracked path listed a directory: %v", listed)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// "new body marker" is exactly as long as "old body marker".
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "old body marker", "new body marker", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("the rewrite must be invisible to a stat for this test to bite: size %d→%d, mtime %v→%v",
			before.Size(), after.Size(), before.ModTime(), after.ModTime())
	}
	if stamp.SameContent() {
		t.Fatal("SameContent missed a rewrite that preserved size and modification time")
	}
	if len(listed) != 0 {
		t.Fatalf("re-checking the tracked path listed a directory: %v", listed)
	}
}
