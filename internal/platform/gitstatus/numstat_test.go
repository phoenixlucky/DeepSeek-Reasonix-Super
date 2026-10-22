package gitstatus

import (
	"strconv"
	"testing"
)

// git spells a binary file's counts "-". Reading that as zero says the file
// changed by nothing, which is a different answer from not knowing.
func TestBinaryCountsStayUnsaidRatherThanZero(t *testing.T) {
	got := ParseNumstatZ([]byte("12\t3\tsrc/main.go\x00-\t-\tlogo.png\x00"))
	text, ok := got["src/main.go"]
	if !ok || text.added == nil || *text.added != 12 || text.removed == nil || *text.removed != 3 {
		t.Fatalf("text file = %+v", text)
	}
	binary, ok := got["logo.png"]
	if !ok {
		t.Fatal("the binary file was dropped from the listing")
	}
	if binary.added != nil || binary.removed != nil {
		t.Fatalf("binary counts = %v/%v, want both unsaid", binary.added, binary.removed)
	}
}

// git diff --numstat -z spells a rename or copy as the counts, a tab, NUL,
// the old path, NUL, then the new path. The counts belong to the new path:
// porcelain status reports that name, and filing both paths would add the
// change twice. A following plain record must survive the extra fields, and
// a zero count is a count — only "-" stays unsaid.
func TestRenameAndCopyCountsStayWithTheDestination(t *testing.T) {
	raw := []byte("2\t0\tkeep.go\x00" +
		"1\t0\t\x00old.go\x00new.go\x00" +
		"-\t-\tlogo.png\x00" +
		"0\t0\t\x00stay.txt\x00copied.txt\x00" +
		"4\t1\t\x00sub/old name.txt\x00sub/new name.txt\x00")
	got := ParseNumstatZ(raw)

	requireCount(t, got, "keep.go", 2, 0)
	requireCount(t, got, "new.go", 1, 0)
	requireCount(t, got, "copied.txt", 0, 0)
	requireCount(t, got, "sub/new name.txt", 4, 1)
	if _, ok := got["logo.png"]; !ok {
		t.Fatal("binary file was dropped")
	}
	if n := got["logo.png"]; n.added != nil || n.removed != nil {
		t.Fatalf("binary counts = %v/%v, want both unsaid", n.added, n.removed)
	}
	for _, path := range []string{"old.go", "stay.txt", "sub/old name.txt"} {
		if _, ok := got[path]; ok {
			t.Fatalf("source path %s was counted separately: %+v", path, got)
		}
	}
	if len(got) != 5 {
		t.Fatalf("got %d records (%v), want the four text paths and the binary", len(got), got)
	}
}

// A torn rename has no destination. It must not be filed under the source
// and must not panic.
func TestTruncatedRenameNumstatIsDropped(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("1\t4\t\x00only-old"),
		[]byte("1\t4\t\x00only-old\x00"),
		[]byte("1\t4\t\x00"),
	} {
		got := ParseNumstatZ(raw)
		if len(got) != 0 {
			t.Fatalf("ParseNumstatZ(%q) = %v, want nothing", raw, got)
		}
	}
}

func requireCount(t *testing.T, got map[string]numstat, path string, added, removed int) {
	t.Helper()
	n, ok := got[path]
	if !ok || n.added == nil || n.removed == nil || *n.added != added || *n.removed != removed {
		addedSeen, removedSeen := "<nil>", "<nil>"
		if n.added != nil {
			addedSeen = strconv.Itoa(*n.added)
		}
		if n.removed != nil {
			removedSeen = strconv.Itoa(*n.removed)
		}
		t.Fatalf("%s = %s/%s present=%v, want %d/%d", path, addedSeen, removedSeen, ok, added, removed)
	}
}
