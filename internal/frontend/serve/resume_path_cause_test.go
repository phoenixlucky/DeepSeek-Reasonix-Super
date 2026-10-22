package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func resumeCodeOf(t *testing.T, err error) string {
	t.Helper()
	var c *coded
	if !errors.As(err, &c) {
		t.Fatalf("resume refused with %v, which carries no code", err)
	}
	return c.reason.Code
}

func resumeFixture(t *testing.T) (*Server, string) {
	t.Helper()
	dir := testenv.TempDir(t)
	active := filepath.Join(dir, "active.jsonl")
	if err := os.WriteFile(active, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: dir, SessionPath: active})
	return New(ctrl, bc, config.ServeConfig{}), dir
}

// A transcript that is gone and a path the host could not read are different
// facts with different next steps; both used to arrive as "cannot resolve the
// path", which tells the person nothing and support less.
func TestResumeSaysTheTranscriptIsGoneRatherThanThePathIsBad(t *testing.T) {
	srv, dir := resumeFixture(t)
	status, err := srv.resumeInto(filepath.Join(dir, "gone.jsonl"))
	if err == nil || status != 404 || resumeCodeOf(t, err) != codeSessionUnknown {
		t.Fatalf("missing transcript = %d, %v; want 404 %s", status, err, codeSessionUnknown)
	}
}

func TestResumeKeepsBadPathForANameThatIsNotATranscript(t *testing.T) {
	srv, dir := resumeFixture(t)
	status, err := srv.resumeInto(filepath.Join(dir, "notes.txt"))
	if err == nil || status != 400 || resumeCodeOf(t, err) != codeSessionBadPath {
		t.Fatalf("non-transcript name = %d, %v; want 400 %s", status, err, codeSessionBadPath)
	}
}

func TestResumeNamesTheOSReasonWhenAPathCannotBeRead(t *testing.T) {
	srv, dir := resumeFixture(t)
	a, b := filepath.Join(dir, "loop-a.jsonl"), filepath.Join(dir, "loop-b.jsonl")
	if err := os.Symlink(b, a); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	status, err := srv.resumeInto(a)
	if err == nil || status != 400 || resumeCodeOf(t, err) != codeSessionPathUnreadable {
		t.Fatalf("symlink loop = %d, %v; want 400 %s", status, err, codeSessionPathUnreadable)
	}
	var c *coded
	_ = errors.As(err, &c)
	if detail, _ := c.reason.Params["detail"].(string); detail == "" {
		t.Fatalf("the refusal carries no OS reason: %+v", c.reason)
	}
}

// A pane whose own session folder was never created cannot hold a transcript
// that exists elsewhere, and saying the path is bad hid that every click on an
// older conversation was being refused for the same reason.
func TestResumeOfAnExistingTranscriptOutsideAMissingSessionDirIsOutside(t *testing.T) {
	srv, dir := resumeFixture(t)
	elsewhere := testenv.TempDir(t)
	file := filepath.Join(elsewhere, "older.jsonl")
	if err := os.WriteFile(file, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	status, err := srv.resumeInto(file)
	if err == nil || status != 403 || resumeCodeOf(t, err) != codeSessionOutside {
		t.Fatalf("transcript beside a missing session dir = %d, %v; want 403 %s", status, err, codeSessionOutside)
	}
}
