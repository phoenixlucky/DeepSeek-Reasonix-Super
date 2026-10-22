// A checkpoint the process cannot read is not a checkpoint that holds nothing:
// while a transcript is missing an image and the only copy sits in a checkpoint
// that cannot be inspected, the rewrite is refused. Only a checkpoint that is
// genuinely absent leaves nothing to protect.
package sessionstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/store"
)

// The audit's red test, kept verbatim: an unreadable checkpoint must not be
// mistaken for one holding no such image.
func TestAuditUnreadableCheckpointStillRefusesDegradedImageRewrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a file the test user cannot read")
	}
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	url := testImageDataURL(21, 4096)
	imageSession(t, path, url)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(store.SessionBlobsDir(path)); err != nil {
		t.Fatal(err)
	}
	stripCheckpointImages(t, path)
	degraded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(unavailableImageKeys(degraded.Snapshot())) == 0 {
		t.Fatal("fixture: the loaded session should be missing the image")
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	// The checkpoint is still the only copy of the image, but this process
	// cannot read it (permission/ACL change after a restore, a foreign owner).
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	err = writeSessionMessages(path, degraded.Snapshot())
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	cp, _ := os.ReadFile(path)
	if survived := bytes.Contains(cp, []byte(url)); !errors.Is(err, ErrSessionImagesUnavailable) || !survived {
		t.Fatalf("rewrite with an unreadable checkpoint = %v (image copy survived: %v), want ErrSessionImagesUnavailable and the file untouched",
			err, survived)
	}
}

// The counter-case the fail-closed rule must not swallow: with no checkpoint
// on disk there is no second copy to protect, so the rewrite proceeds.
func TestSessionAbsentCheckpointStillRewrites(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	url := testImageDataURL(22, 4096)
	imageSession(t, path, url)
	if err := os.RemoveAll(store.SessionBlobsDir(path)); err != nil {
		t.Fatal(err)
	}
	stripCheckpointImages(t, path)
	degraded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(unavailableImageKeys(degraded.Snapshot())) == 0 {
		t.Fatal("fixture: the loaded session should be missing the image")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if err := writeSessionMessages(path, degraded.Snapshot()); err != nil {
		t.Fatalf("rewrite with no checkpoint on disk = %v, want it to proceed", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the rewrite did not write the transcript: %v", err)
	}
}

// A refused rewrite leaves every byte of the checkpoint where it was: the
// refusal itself must not count as a way to touch the file.
func TestSessionUnreadableCheckpointRefusalLeavesBytesIdentical(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a file the test user cannot read")
	}
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	url := testImageDataURL(23, 4096)
	imageSession(t, path, url)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(store.SessionBlobsDir(path)); err != nil {
		t.Fatal(err)
	}
	stripCheckpointImages(t, path)
	degraded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	err = writeSessionMessages(path, degraded.Snapshot())
	if chmodErr := os.Chmod(path, 0o600); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if !errors.Is(err, ErrSessionImagesUnavailable) {
		t.Fatalf("rewrite with an unreadable checkpoint = %v, want ErrSessionImagesUnavailable", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("a refused rewrite changed the checkpoint's bytes")
	}
}
