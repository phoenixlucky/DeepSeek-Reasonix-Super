package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// A paste a person chose to keep out of the conversation becomes a plain text
// file under the same attachment directory as pictures, and what goes into the
// message is a pointer, never the text.
func TestSaveTextAttachmentStoresThePasteAndPointsAtIt(t *testing.T) {
	root := testenv.TempDir(t)
	body := strings.Repeat("line of a very long log\n", 400)
	rel, err := SaveTextAttachmentInRoot(root, body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rel, ".reasonix/attachments/") || !strings.HasSuffix(rel, ".txt") {
		t.Fatalf("stored at %q, want a .txt under .reasonix/attachments", rel)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || string(got) != body {
		t.Fatalf("stored text differs from the paste (err %v)", err)
	}
	pointer := TextAttachmentPointer(rel, body)
	for _, want := range []string{rel, "400 lines", "read_file"} {
		if !strings.Contains(pointer, want) {
			t.Fatalf("pointer %q lacks %q", pointer, want)
		}
	}
	if strings.Contains(pointer, "very long log") || len(pointer) > 600 {
		t.Fatalf("the pointer carries the text or is not small: %d bytes", len(pointer))
	}
}

func TestSaveTextAttachmentRefusesWhatIsNotText(t *testing.T) {
	root := testenv.TempDir(t)
	for name, tc := range map[string]struct {
		in   string
		want error
	}{
		"empty":        {"", ErrAttachmentEmpty},
		"invalid utf8": {"ok \xff\xfe broken", ErrAttachmentNotText},
		"nul byte":     {"binary\x00data", ErrAttachmentNotText},
		"too large":    {strings.Repeat("x", MaxTextAttachmentBytes+1), ErrAttachmentTooLarge},
	} {
		if _, err := SaveTextAttachmentInRoot(root, tc.in); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".reasonix", "attachments"))
	if len(entries) != 0 {
		t.Fatalf("a refused paste left %d files behind", len(entries))
	}
}

func TestSaveTextAttachmentKeepsTheDirectoryRules(t *testing.T) {
	root := testenv.TempDir(t)
	outside := testenv.TempDir(t)
	if err := os.MkdirAll(filepath.Join(root, ".reasonix"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".reasonix", "attachments")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, err := SaveTextAttachmentInRoot(root, "hello"); err == nil {
		t.Fatal("a paste was written through a symlinked attachment directory")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("a file landed outside the workspace: %v", entries)
	}
}
