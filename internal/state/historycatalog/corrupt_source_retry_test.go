package historycatalog

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/store"
)

// requireDeniableReads skips a permission fixture on hosts where chmod cannot
// deny a read: Windows maps 000 to the read-only attribute, and root ignores
// the mode bits entirely.
func requireDeniableReads(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 cannot deny a read on this platform/user")
	}
}

// openTempCatalog keeps every probe off the user cache: the catalog always
// points at a disposable temp path, and never resolves DefaultPath.
func openTempCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := Open(context.Background(), Options{
		Path: filepath.Join(testenv.TempDir(t), "history.sqlite"), InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	return catalog
}

func sourceHealth(t *testing.T, catalog *Catalog, path string) string {
	t.Helper()
	var health string
	if err := catalog.db.QueryRow(`SELECT health FROM history_sources WHERE path=?`, path).Scan(&health); err != nil {
		t.Fatalf("read health for %s: %v", path, err)
	}
	return health
}

func sourceLastError(t *testing.T, catalog *Catalog, path string) string {
	t.Helper()
	var lastError string
	if err := catalog.db.QueryRow(`SELECT last_error FROM history_sources WHERE path=?`, path).Scan(&lastError); err != nil {
		t.Fatalf("read last_error for %s: %v", path, err)
	}
	return lastError
}

func rootScan(t *testing.T, catalog *Catalog, root string) (state string, indexed, total, generation int64) {
	t.Helper()
	if err := catalog.db.QueryRow(`SELECT state,indexed,total,scan_generation FROM history_roots WHERE path=?`, root).
		Scan(&state, &indexed, &total, &generation); err != nil {
		t.Fatalf("read history_roots row for %s: %v", root, err)
	}
	return state, indexed, total, generation
}

func sourceDocumentIDs(t *testing.T, catalog *Catalog, path string) []int64 {
	t.Helper()
	rows, err := catalog.db.Query(`SELECT id FROM history_documents WHERE source_path=? ORDER BY id`, path)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// A transcript that could not be read is recorded as corrupt and its root is
// stamped ready with the failing signature. The reconcile short-circuit trusts
// that signature, so once the transcript becomes readable again (wrong mode
// after a restore, a mount that came back) nothing revalidates it: only a save
// event or the manual `reasonix catalogs reindex history` retries it.
func TestCorruptSourceIsRetriedOnceTheTranscriptIsReadable(t *testing.T) {
	requireDeniableReads(t)
	ctx := context.Background()
	root := testenv.TempDir(t)
	path := filepath.Join(root, "restored.jsonl")
	saveMessages(t, path, provider.Message{Role: provider.RoleUser, Content: "zephyr restored decision"})
	logPath := store.SessionEventLog(path)
	if err := os.Chmod(logPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(logPath, 0o600) })

	catalog := openTempCatalog(t)
	target := Root{Path: root, Source: "project", Scope: "project", WorkspaceRoot: root}
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	if health := sourceHealth(t, catalog, path); health != "corrupt" {
		t.Fatalf("fixture: unreadable transcript recorded as health=%q, want %q", health, "corrupt")
	}
	state, indexed, total, _ := rootScan(t, catalog, root)
	if state != "ready" || indexed == total {
		t.Fatalf("fixture: root state=%q indexed=%d total=%d, want a ready row that hides a failed file", state, indexed, total)
	}

	// The fault heals. Nothing about the files changed, only whether they can
	// be read — which is exactly the fact the projection got wrong.
	if err := os.Chmod(logPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	if health := sourceHealth(t, catalog, path); health != "ok" {
		t.Fatalf("transcript is readable again but the projection still says health=%q: the scan skipped the root "+
			"because its size/mtime signature and ready state had not changed, so nothing ever asks whether the read works", health)
	}
	state, indexed, total, _ = rootScan(t, catalog, root)
	if state != "ready" || indexed != total {
		t.Fatalf("after the fault healed: root state=%q indexed=%d total=%d, want a complete ready row", state, indexed, total)
	}
	got, err := catalog.Search(ctx, SearchRequest{Query: "zephyr restored", Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("search after the fault healed = %d item(s), want the recovered session", len(got.Items))
	}
	var preview string
	if err := catalog.db.QueryRow(`SELECT preview FROM history_sources WHERE path=?`, path).Scan(&preview); err != nil {
		t.Fatal(err)
	}
	if preview == "" {
		t.Fatalf("recovered row carries no preview: the readable transcript was not the content that got indexed")
	}
}

// Control: the fix must not turn every periodic tick into a re-read of every
// transcript. A root whose last scan was complete and healthy still
// short-circuits on the size/mtime signature, before any transcript is opened.
func TestHealthyUnchangedRootIsSkippedWithoutReadingTranscripts(t *testing.T) {
	ctx := context.Background()
	root := testenv.TempDir(t)
	path := filepath.Join(root, "healthy.jsonl")
	saveMessages(t, path, provider.Message{Role: provider.RoleUser, Content: "zephyr healthy marker"})
	catalog := openTempCatalog(t)
	target := Root{Path: root, Source: "project", Scope: "project", WorkspaceRoot: root}
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	_, indexed, total, generation := rootScan(t, catalog, root)
	if indexed != 1 || total != 1 {
		t.Fatalf("fixture: indexed=%d total=%d, want a complete root", indexed, total)
	}
	revision := catalog.revision.Load()
	documentIDs := sourceDocumentIDs(t, catalog, path)

	// A second scan with nothing changed must do no work: no new generation,
	// no revision bump, and no document row churn (a reindex rewrites ids).
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	assertRootSkipped(t, catalog, root, path, generation, revision, documentIDs)
	if health := sourceHealth(t, catalog, path); health != "ok" {
		t.Fatalf("healthy source health=%q after a skip", health)
	}

	// Stronger proof where reads can be denied: make the transcript unreadable
	// after the scan. A rescan would stamp corrupt; the fast path never opens
	// it, so health stays ok and the indexed terms still answer searches.
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	logPath := store.SessionEventLog(path)
	if err := os.Chmod(logPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(logPath, 0o600) })
	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	assertRootSkipped(t, catalog, root, path, generation, revision, documentIDs)
	if health := sourceHealth(t, catalog, path); health != "ok" {
		t.Fatalf("unchanged healthy root was re-read: a rescan that opened the now-unreadable transcript would "+
			"have stamped health=corrupt, got health=%q", health)
	}
	got, err := catalog.Search(ctx, SearchRequest{Query: "zephyr healthy", Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("skip lost the already-indexed terms: search returned %d item(s)", len(got.Items))
	}
}

func assertRootSkipped(t *testing.T, catalog *Catalog, root, path string, generation int64, revision uint64, documentIDs []int64) {
	t.Helper()
	_, _, _, afterGeneration := rootScan(t, catalog, root)
	if afterGeneration != generation {
		t.Fatalf("root was rescanned: scan_generation went %d -> %d, want the fast-path skip", generation, afterGeneration)
	}
	if afterRevision := catalog.revision.Load(); afterRevision != revision {
		t.Fatalf("rescan published a revision: %d -> %d, want the fast-path skip", revision, afterRevision)
	}
	if afterIDs := sourceDocumentIDs(t, catalog, path); !slices.Equal(afterIDs, documentIDs) {
		t.Fatalf("document rows were rewritten: %v -> %v, want the fast-path skip", documentIDs, afterIDs)
	}
}

// Control: a corrupt source that STAYS corrupt is retried by every scan, not
// frozen as "ready". Bounded: one read attempt per corrupt source per scan (the
// tick is five minutes), each scan bumps scan_generation exactly once, and
// healthy siblings are not re-read. The undecodable fixture has no ledger
// digest, so it also covers sources that look skippable from size/mtime alone.
func TestCorruptSourceStaysCorruptIsRetriedOnEveryScan(t *testing.T) {
	ctx := context.Background()
	root := testenv.TempDir(t)
	goodPath := filepath.Join(root, "good.jsonl")
	saveMessages(t, goodPath, provider.Message{Role: provider.RoleUser, Content: "zephyr good marker"})
	badPath := filepath.Join(root, "bad.jsonl")
	if err := os.WriteFile(badPath, []byte("this is not a session transcript\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := openTempCatalog(t)
	target := Root{Path: root, Source: "project", Scope: "project", WorkspaceRoot: root}

	if err := catalog.ReconcileRoot(ctx, target); err != nil {
		t.Fatal(err)
	}
	state, indexed, total, generation := rootScan(t, catalog, root)
	if state != "ready" || indexed != 1 || total != 2 {
		t.Fatalf("fixture: state=%q indexed=%d total=%d, want one healthy and one corrupt source", state, indexed, total)
	}
	if health := sourceHealth(t, catalog, badPath); health != "corrupt" {
		t.Fatalf("fixture: undecodable transcript recorded as health=%q, want corrupt", health)
	}
	goodIDs := sourceDocumentIDs(t, catalog, goodPath)
	if len(goodIDs) == 0 {
		t.Fatal("fixture: healthy sibling has no indexed documents")
	}

	for scan := 2; scan <= 3; scan++ {
		if err := catalog.ReconcileRoot(ctx, target); err != nil {
			t.Fatal(err)
		}
		state, indexed, total, afterGeneration := rootScan(t, catalog, root)
		if afterGeneration != generation+1 {
			t.Fatalf("scan %d: scan_generation went %d -> %d, want exactly one retry pass (the root must not be skipped)",
				scan, generation, afterGeneration)
		}
		generation = afterGeneration
		if state != "ready" || indexed != 1 || total != 2 {
			t.Fatalf("scan %d: state=%q indexed=%d total=%d, want the corrupt source still retried and unindexed",
				scan, state, indexed, total)
		}
		if health := sourceHealth(t, catalog, badPath); health != "corrupt" {
			t.Fatalf("scan %d: still-unreadable transcript health=%q, want corrupt", scan, health)
		}
		if lastError := sourceLastError(t, catalog, badPath); lastError == "" {
			t.Fatalf("scan %d: corrupt source has no last_error, want the retry's failure recorded", scan)
		}
		if afterIDs := sourceDocumentIDs(t, catalog, goodPath); !slices.Equal(afterIDs, goodIDs) {
			t.Fatalf("scan %d: healthy sibling was re-read/reindexed: %v -> %v", scan, goodIDs, afterIDs)
		}
		if badIDs := sourceDocumentIDs(t, catalog, badPath); len(badIDs) != 0 {
			t.Fatalf("scan %d: corrupt source indexed %v, want no documents", scan, badIDs)
		}
	}

	// The user-facing counters stay honest: incomplete, not silently complete.
	status := catalog.Status()
	if status.Indexed != 1 || status.Total != 2 || status.Failed != 1 {
		t.Fatalf("status indexed=%d total=%d failed=%d, want 1/2 with one failed source", status.Indexed, status.Total, status.Failed)
	}
}
