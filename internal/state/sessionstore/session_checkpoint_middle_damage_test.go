package sessionstore

// A checkpoint can be damaged in the MIDDLE: a record that will not decode with
// valid records after it. Only a torn FINAL record may be recovered, because a
// prefix kept across a break drops every record after it.

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
)

// checkpointLines writes a checkpoint of three turns and returns its path and
// the file's records as written.
func checkpointLines(t *testing.T) (path string, lines [][]byte) {
	t.Helper()
	dir := testenv.TempDir(t)
	path = filepath.Join(dir, "session.jsonl")
	live := NewSession("sys")
	for _, text := range []string{"第一句话", "第二句话", "第三句话"} {
		live.Add(provider.Message{Role: provider.RoleUser, Content: text, RawContent: text})
		live.Add(provider.Message{Role: provider.RoleAssistant, Content: "a"})
	}
	if err := live.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == '\n' {
			if i > start {
				lines = append(lines, raw[start:i])
			}
			start = i + 1
		}
	}
	if len(lines) < 6 {
		t.Fatalf("the checkpoint has %d records, want at least 6 for this test to mean anything", len(lines))
	}
	return path, lines
}

func writeCheckpointLines(t *testing.T, path string, lines [][]byte) {
	t.Helper()
	out := make([]byte, 0, 1024)
	for _, l := range lines {
		out = append(out, l...)
		out = append(out, '\n')
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCheckpointDamagedInTheMiddleIsNotRecoverable: the record that fails is not
// the last one, so nothing may be kept from it.
func TestCheckpointDamagedInTheMiddleIsNotRecoverable(t *testing.T) {
	path, lines := checkpointLines(t)
	lines[3] = []byte(`{"role":"user","content":`) // truncated, with two intact records after it
	writeCheckpointLines(t, path, lines)

	msgs := loadSessionMessagesFromJSONLKeepingDecodedTail(path)
	if len(msgs) != 0 {
		t.Fatalf("kept %d records across a mid-file break (users = %q): a record that will not decode in the middle means the prefix cannot be trusted",
			len(msgs), sessionUserTexts(msgs))
	}
	if _, err := loadSessionMessagesFromJSONL(path); err == nil {
		t.Fatal("the strict loader accepted a checkpoint damaged in the middle")
	}
}

// TestCheckpointTornFinalRecordIsStillRecoverable: the branch's own reason for
// tolerating a decode error — a half-written final record — must keep working.
func TestCheckpointTornFinalRecordIsStillRecoverable(t *testing.T) {
	path, lines := checkpointLines(t)
	lines[len(lines)-1] = []byte(`{"role":"assistant","content":`) // half-written last record
	writeCheckpointLines(t, path, lines)

	msgs := loadSessionMessagesFromJSONLKeepingDecodedTail(path)
	if got := sessionUserTexts(msgs); len(got) != 3 {
		t.Fatalf("users = %q, want the three turns the intact records still hold", got)
	}
}
