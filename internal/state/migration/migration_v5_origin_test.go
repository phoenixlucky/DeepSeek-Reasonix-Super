package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

func writeSourceHeader(t *testing.T, root, id, cwd string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"origin": "new", "cwd": cwd})
	dir := filepath.Join(root, "desktop-sessions-v5", "by-id", id)
	if err := os.WriteFile(filepath.Join(dir, "header.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sessionsIn(t *testing.T, project string) []string {
	t.Helper()
	found, _ := filepath.Glob(filepath.Join(config.ProjectSessionDir(project), v4Prefix+"*.jsonl"))
	var own []string
	for _, f := range found {
		if strings.Count(filepath.Base(f), ".") == 1 {
			own = append(own, f)
		}
	}
	return own
}

// A source conversation says where it was made. With no registry entry for it, that
// is what files it, and the project the person happened to pick is not involved.
func TestV5SessionsAreFiledByTheirOwnHeaderNotByTheChosenProject(t *testing.T) {
	home := isolateMigrationHome(t)
	root := filepath.Join(home, "Roaming", "reasonix")
	book, wsA, wsB := filepath.Join(home, "book"), filepath.Join(home, "wsA"), filepath.Join(home, "wsB")
	for _, d := range []string{book, wsA, wsB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a, b := "00000000000000000000000000000011", "desktop-manual-00000000000000000000000000000012"
	store := filepath.Join(root, "desktop-sessions-v5", "by-id")
	seedV4Conversation(t, store, a)
	seedV4Conversation(t, store, b)
	writeSourceHeader(t, root, a, wsA)
	writeSourceHeader(t, root, b, wsB)

	res := RunLegacySessionImportInto(root, config.ProjectSessionDir(book), event.Discard)
	if got := totalImported(res.SessionImports); got != 2 || len(res.SessionErrs) != 0 {
		t.Fatalf("imported %d, errs %v; want 2", got, res.SessionErrs)
	}
	if n := len(sessionsIn(t, book)); n != 0 {
		t.Fatalf("%d conversations were written into the project the person picked; none of them were made there", n)
	}
	if n := len(sessionsIn(t, wsA)); n != 1 {
		t.Fatalf("project A holds %d, want its own 1", n)
	}
	if n := len(sessionsIn(t, wsB)); n != 1 {
		t.Fatalf("project B holds %d, want its own 1", n)
	}
	if len(res.Workspaces) != 2 {
		t.Fatalf("routed projects = %v, want both listed so their conversations can be reached", res.Workspaces)
	}
}

// A registry that does not list a conversation does not decide for it either.
func TestARegistryThatLacksASessionDoesNotSendItToTheChosenProject(t *testing.T) {
	home := isolateMigrationHome(t)
	root := filepath.Join(home, "Roaming", "reasonix")
	book, wsA, other := filepath.Join(home, "book"), filepath.Join(home, "wsA"), filepath.Join(home, "other")
	for _, d := range []string{book, wsA, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	listed, unlisted := "00000000000000000000000000000021", "00000000000000000000000000000022"
	store := filepath.Join(root, "desktop-sessions-v5", "by-id")
	seedV4Conversation(t, store, listed)
	seedV4Conversation(t, store, unlisted)
	writeDesktopRegistry(t, root, map[string][]string{other: {listed}})
	writeSourceHeader(t, root, unlisted, wsA)

	RunLegacySessionImportInto(root, config.ProjectSessionDir(book), event.Discard)
	if n := len(sessionsIn(t, book)); n != 0 {
		t.Fatalf("%d conversations landed in the picked project", n)
	}
	if len(sessionsIn(t, wsA)) != 1 || len(sessionsIn(t, other)) != 1 {
		t.Fatalf("the header-filed and the registry-filed conversations are not each in their own project")
	}
}

// A conversation that names no project at all is not given one: it stays in the
// source, untouched, and the result says so.
func TestAV5SessionThatNamesNoProjectIsSkippedAndSaid(t *testing.T) {
	home := isolateMigrationHome(t)
	root := filepath.Join(home, "Roaming", "reasonix")
	book := filepath.Join(home, "book")
	if err := os.MkdirAll(book, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "00000000000000000000000000000031"
	seedV4Conversation(t, filepath.Join(root, "desktop-sessions-v5", "by-id"), id)
	before := hashTree(t, root)

	res := RunLegacySessionImportInto(root, config.ProjectSessionDir(book), event.Discard)
	if totalImported(res.SessionImports) != 0 || len(sessionsIn(t, book)) != 0 {
		t.Fatalf("a conversation with no origin was written: %+v", res.SessionImports)
	}
	if len(res.SessionSkips) != 1 || res.SessionSkips[0].Reason != "no_project" {
		t.Fatalf("skips = %+v, want one with reason no_project", res.SessionSkips)
	}
	if hashTree(t, root) != before {
		t.Fatal("the source was modified")
	}
}

// Running it again writes nothing new: each conversation is already in its project.
func TestImportingAgainDoesNotWriteTheSameConversationsTwice(t *testing.T) {
	home := isolateMigrationHome(t)
	root := filepath.Join(home, "Roaming", "reasonix")
	book, wsA := filepath.Join(home, "book"), filepath.Join(home, "wsA")
	for _, d := range []string{book, wsA} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	id := "00000000000000000000000000000041"
	seedV4Conversation(t, filepath.Join(root, "desktop-sessions-v5", "by-id"), id)
	writeSourceHeader(t, root, id, wsA)
	RunLegacySessionImportInto(root, config.ProjectSessionDir(book), event.Discard)
	if res := RunLegacySessionImportInto(root, config.ProjectSessionDir(book), event.Discard); totalImported(res.SessionImports) != 0 {
		t.Fatalf("a second run imported %+v", res.SessionImports)
	}
	if n := len(sessionsIn(t, wsA)) + len(sessionsIn(t, book)); n != 1 {
		t.Fatalf("%d copies exist after two runs, want 1", n)
	}
}
