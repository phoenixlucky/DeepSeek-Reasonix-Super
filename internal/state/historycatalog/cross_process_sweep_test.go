package historycatalog

import (
	"context"
	"os"
	"path/filepath"
	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
	"testing"
	"time"
)

func TestSweepStillRetiresTranscriptThatIsGone(t *testing.T) {
	ctx := context.Background()
	root := testenv.TempDir(t)
	dbPath := filepath.Join(testenv.TempDir(t), "history.sqlite")

	clock := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	catalog, err := Open(ctx, Options{Path: dbPath, ReconcileInterval: time.Hour, MissingGrace: time.Millisecond, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	target := Root{Path: root, Source: "project", Scope: "project", WorkspaceRoot: root}

	gone := filepath.Join(root, "gone.jsonl")
	saveMessages(t, gone, provider.Message{Role: provider.RoleUser, Content: "zephyr removed conversation"})
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	if hits, err := catalog.Search(ctx, SearchRequest{Query: "zephyr removed", Roots: []string{root}}); err != nil || len(hits.Items) != 1 {
		t.Fatalf("fixture: search hits=%d err=%v, want the indexed conversation", len(hits.Items), err)
	}
	for _, path := range []string{gone, sessionstore.BranchMetaPath(gone)} {
		_ = os.Remove(path)
	}

	// First scan marks it missing inside the grace window.
	clock = clock.Add(time.Second)
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	if health := sourceHealthOf(t, catalog, gone); health != "missing" {
		t.Fatalf("removed source health=%q, want missing inside the grace window", health)
	}
	// A later scan past the grace retires the rows.
	clock = clock.Add(time.Second)
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := catalog.db.QueryRow(`SELECT COUNT(*) FROM history_sources WHERE path=?`, gone).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("removed source still has %d row(s) after the grace window", count)
	}
	if docs := sourceDocumentCount(t, catalog, gone); docs != 0 {
		t.Fatalf("removed source kept %d document row(s)", docs)
	}
}

func sourceHealthOf(t *testing.T, catalog *Catalog, path string) string {
	t.Helper()
	var health string
	if err := catalog.db.QueryRow(`SELECT health FROM history_sources WHERE path=?`, path).Scan(&health); err != nil {
		t.Fatalf("read health for %s: %v", path, err)
	}
	return health
}

func sourceDocumentCount(t *testing.T, catalog *Catalog, path string) int {
	t.Helper()
	var count int
	if err := catalog.db.QueryRow(`SELECT COUNT(*) FROM history_documents WHERE source_path=?`, path).Scan(&count); err != nil {
		t.Fatalf("count documents for %s: %v", path, err)
	}
	return count
}
