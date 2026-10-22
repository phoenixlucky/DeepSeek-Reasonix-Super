package sessionstore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/state/sessionv4/v4fixture"
)

// A 1.x conversation whose own listing cache is absent is replayed once, not
// on every listing: a folder of thousands re-read them all each time it opened.
func TestV4ConversationsWithoutAListingCacheAreReplayedOnce(t *testing.T) {
	s := v4fixture.New(t)
	sessions := filepath.Join(filepath.Dir(s.Root), "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for i := range 3 {
		dir := s.Session(fmt.Sprintf("desktop-%024x", i+1), 3)
		s.Batch(dir, v4fixture.Ended,
			v4fixture.Event{Kind: "message/complete", Payload: v4fixture.Msg("u", "user", fmt.Sprintf("question %d", i))},
			v4fixture.Event{Kind: "message/complete", Payload: v4fixture.Msg("a", "assistant", "answer")})
		dirs = append(dirs, dir)
	}
	first, _ := ListSessions(sessions)
	if len(first) != len(dirs) {
		t.Fatalf("first listing has %d rows, want %d", len(first), len(dirs))
	}
	// Each log keeps its size and modification time but no longer reads: a
	// listing that replays a conversation loses its row, one that remembered it does not.
	for _, dir := range dirs {
		log := filepath.Join(dir, "events.frames")
		info, err := os.Stat(log)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(log, bytes.Repeat([]byte{0}, int(info.Size())), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(log, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
	}
	second, _ := ListSessions(sessions)
	if len(second) != len(first) {
		t.Fatalf("second listing has %d rows against %d: it replayed the conversations again", len(second), len(first))
	}
	previews := map[string]string{}
	for _, row := range first {
		previews[row.Path] = row.Preview
	}
	for _, row := range second {
		if previews[row.Path] != row.Preview {
			t.Fatalf("%s previews %q, first listing said %q", row.Path, row.Preview, previews[row.Path])
		}
	}
	if entries := len(loadV4Listings(sessions).Entries); entries != len(dirs) {
		t.Fatalf("persisted entries = %d, want %d", entries, len(dirs))
	}
}
