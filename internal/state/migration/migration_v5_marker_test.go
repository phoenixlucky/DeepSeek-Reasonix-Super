package migration

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

// Desktop 1.x names its by-id sessions with 24 hex digits. The destination
// already holding the empty markers a flat legacy pass leaves must not stop
// them importing, and importing again must find nothing new rather than fail.
func TestDesktopV5SessionsImportBesideEmptyLegacyMarkers(t *testing.T) {
	home := isolateMigrationHome(t)
	root := filepath.Join(home, "Roaming", "reasonix")
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	ids := []string{"727525f3e44fa790368ca635", "0a1b2c3d4e5f60718293a4b5"}
	for _, id := range ids {
		seedV4Conversation(t, filepath.Join(root, "desktop-sessions-v5", "by-id"), id)
		writeSourceHeader(t, root, id, ws)
	}
	dest := config.ProjectSessionDir(ws)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{".legacy-imported", ".legacy-imported.explicit.096bf5488d83ee6b", ".legacy-imported.explicit.096bf5488d83ee6b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dest, marker), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	first := RunLegacySessionImportInto(root, dest, event.Discard)
	if got := totalImported(first.SessionImports); got != len(ids) || len(first.SessionErrs) != 0 || len(first.SessionSkips) != 0 {
		t.Fatalf("first run: imported %d, errs %v, skips %v; want %d imported", got, first.SessionErrs, first.SessionSkips, len(ids))
	}
	second := RunLegacySessionImportInto(root, dest, event.Discard)
	if got := totalImported(second.SessionImports); got != 0 || len(second.SessionErrs) != 0 {
		t.Fatalf("second run: imported %d, errs %v; want a clean no-op", got, second.SessionErrs)
	}
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(dest, v4Prefix+id+".jsonl")); err != nil {
			t.Errorf("session %s missing after the retry: %v", id, err)
		}
	}
}
