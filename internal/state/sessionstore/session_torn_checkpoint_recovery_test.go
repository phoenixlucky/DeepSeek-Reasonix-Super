package sessionstore

// A crash can tear the checkpoint too: the in-place append that keeps the read
// model current is the write an unclean exit leaves half done, and the
// checkpoint's intact lines still hold the turns the torn event log dropped.

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
)

// The failure mode: the log lost the record describing the second turn and the
// checkpoint's own last line never finished being written. Reading the tear as
// "this checkpoint does not extend the log" returned the shorter replay with
// err == nil, and the next save rewrote both files from it — so the turns the
// checkpoint's intact lines still held were gone for good.
func TestTornCheckpointTailDoesNotHideTheTurnsItHolds(t *testing.T) {
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "session.jsonl")
	live := NewSession("sys")
	live.Add(provider.Message{Role: provider.RoleUser, Content: "第一句话", RawContent: "第一句话"})
	live.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})
	if err := live.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	live.Add(provider.Message{Role: provider.RoleUser, Content: "第二句话", RawContent: "第二句话"})
	live.Add(provider.Message{Role: provider.RoleAssistant, Content: "ok"})
	if err := live.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	tearEventLogTail(t, path, 20)
	cp, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, cp[:len(cp)-20], 0o600); err != nil {
		t.Fatal(err)
	}

	msgs, _, damaged, err := loadSessionMessages(path)
	if err != nil {
		t.Fatalf("open with a torn checkpoint tail: %v", err)
	}
	if !damaged {
		t.Fatal("want the torn log reported as damaged so the next save heals it")
	}
	if got := sessionUserTexts(msgs); len(got) != 2 || got[1] != "第二句话" {
		t.Fatalf("users = %q (damaged=%v, err=%v), want the two turns the checkpoint's intact lines still hold",
			got, damaged, err)
	}
}

// Mirror control: decoding a torn checkpoint tolerantly keeps records, it does
// not make a foreign transcript extend this one. Its intact lines contradict
// the prefix the log still replays, so it stays refused even though it is
// longer and even though its own tail is torn too.
func TestTornCheckpointThatContradictsTheLogIsStillRefused(t *testing.T) {
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "session.jsonl")
	live := NewSession("sys")
	live.Add(provider.Message{Role: provider.RoleUser, Content: "第一句话", RawContent: "第一句话"})
	live.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi"})
	if err := live.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	live.Add(provider.Message{Role: provider.RoleUser, Content: "第二句话", RawContent: "第二句话"})
	if err := live.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	tearEventLogTail(t, path, 20)

	if err := writeSessionMessages(path, []provider.Message{
		{Role: provider.RoleSystem, Content: "sys"},
		{Role: provider.RoleUser, Content: "别处写进来的一句", RawContent: "别处写进来的一句"},
		{Role: provider.RoleAssistant, Content: "no"},
		{Role: provider.RoleUser, Content: "以及另一句", RawContent: "以及另一句"},
		{Role: provider.RoleAssistant, Content: "ok"},
	}); err != nil {
		t.Fatal(err)
	}
	foreign, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, foreign[:len(foreign)-20], 0o600); err != nil {
		t.Fatal(err)
	}

	msgs, _, damaged, err := loadSessionMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if !damaged {
		t.Fatal("want the torn log still reported as damaged")
	}
	if got := sessionUserTexts(msgs); len(got) != 1 || got[0] != "第一句话" {
		t.Fatalf("users = %q, want only the prefix the surviving log replays", got)
	}
}
