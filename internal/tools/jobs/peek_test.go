package jobs

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
)

// A job that has written some output and is held open until the test ends.
func runningJob(t *testing.T, m *Manager, write string) *Job {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{})
	j := m.StartForSession("session", "bash", "tail -f log", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = io.WriteString(out, write)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return "", nil
	})
	<-started
	return j
}

func newPeekManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(event.Discard)
	t.Cleanup(m.Close)
	m.SetActiveSessionPath("session", filepath.Join(testenv.TempDir(t), "session.jsonl"))
	return m
}

func TestPeekShowsAJobsOutputWithoutTakingItFromTheModel(t *testing.T) {
	m := newPeekManager(t)
	j := runningJob(t, m, "line one\nline two\n")

	first, ok := m.PeekForSession("session", j.ID, 4096)
	if !ok || first.Text != "line one\nline two\n" || first.Truncated {
		t.Fatalf("peek = %+v ok=%v", first, ok)
	}
	if again, _ := m.PeekForSession("session", j.ID, 4096); again.Text != first.Text {
		t.Fatalf("a second peek read %q, want the same output", again.Text)
	}
	// What the person looked at is still unread by the model's own reader.
	text, _, _ := m.OutputForSession("session", j.ID)
	if text != "line one\nline two\n" {
		t.Fatalf("bash_output after a peek read %q, want everything still unread", text)
	}
}

func TestPeekKeepsTheEndOfALongOutputAndSaysSo(t *testing.T) {
	m := newPeekManager(t)
	j := runningJob(t, m, strings.Repeat("a", 3000)+"END")

	got, ok := m.PeekForSession("session", j.ID, 100)
	if !ok || !got.Truncated || len(got.Text) != 100 || !strings.HasSuffix(got.Text, "END") {
		t.Fatalf("peek len=%d truncated=%v suffix=%q", len(got.Text), got.Truncated, got.Text[max(0, len(got.Text)-5):])
	}
	if got.Bytes != 3003 {
		t.Fatalf("total bytes = %d, want 3003", got.Bytes)
	}
}

func TestPeekNeverStartsInTheMiddleOfACharacter(t *testing.T) {
	m := newPeekManager(t)
	j := runningJob(t, m, strings.Repeat("字", 200))
	for limit := 50; limit < 60; limit++ {
		got, _ := m.PeekForSession("session", j.ID, limit)
		if !utf8.ValidString(got.Text) {
			t.Fatalf("limit %d: peek is not valid UTF-8: %q", limit, got.Text)
		}
	}
}

func TestPeekIsRefusedForAnotherSessionsJobAndAnUnknownID(t *testing.T) {
	m := newPeekManager(t)
	j := runningJob(t, m, "x")
	if _, ok := m.PeekForSession("other", j.ID, 100); ok {
		t.Fatal("a job of another session was peeked")
	}
	if _, ok := m.PeekForSession("session", "no-such-job", 100); ok {
		t.Fatal("an unknown job was peeked")
	}
}

func TestPeekReadsTheArtifactOfAJobWhoseOutputSpilledToDisk(t *testing.T) {
	m := newPeekManager(t)
	j := runningJob(t, m, strings.Repeat("x", defaultTailBytes+4096)+"TAIL")
	got, ok := m.PeekForSession("session", j.ID, 64)
	if !ok || !strings.HasSuffix(got.Text, "TAIL") || !got.Truncated {
		t.Fatalf("peek = %q truncated=%v ok=%v", got.Text, got.Truncated, ok)
	}
}
