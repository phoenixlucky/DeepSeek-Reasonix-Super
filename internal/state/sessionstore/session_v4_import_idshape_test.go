package sessionstore

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/state/sessionv4/v4fixture"
)

func v4StoreWith(t *testing.T, id string) (*v4fixture.Store, string) {
	t.Helper()
	s := v4fixture.New(t)
	dir := s.Session(id, 3)
	s.Batch(dir, v4fixture.Ended,
		v4fixture.Event{Kind: "message/complete", Payload: v4fixture.Msg("m1", "system", "sys")},
		v4fixture.Event{Kind: "message/complete", Payload: v4fixture.Msg("m2", "user", "hello")},
		v4fixture.Event{Kind: "message/complete", Payload: v4fixture.Msg("m3", "assistant", "hi")},
	)
	return s, filepath.Join(filepath.Dir(s.Root), "sessions")
}

// 1.x Desktop names its by-id sessions with 24 hex digits, not 32.
func TestImportV4AcceptsADesktopSessionID(t *testing.T) {
	const id = "727525f3e44fa790368ca635"
	s, sessions := v4StoreWith(t, id)
	n, skipped, err := ImportV4From(os.DirFS(s.Root), sessions, nil, nil)
	if err != nil || n != 1 || len(skipped) != 0 {
		t.Fatalf("imported %d, skipped %+v, err %v; want the session imported", n, skipped, err)
	}
	if _, err := os.Stat(v4ImportPath(sessions, id)); err != nil {
		t.Fatal(err)
	}
}

// A session whose id cannot name a file is reported, never dropped silently.
func TestImportV4ReportsAnIDItCannotUse(t *testing.T) {
	s, sessions := v4StoreWith(t, "not-a-hex-id")
	n, skipped, err := ImportV4From(os.DirFS(s.Root), sessions, nil, nil)
	if err != nil || n != 0 || len(skipped) != 1 || skipped[0].Reason != SkipUnreadableFormat {
		t.Fatalf("imported %d, skipped %+v, err %v; want one unreadable_format skip", n, skipped, err)
	}
}
