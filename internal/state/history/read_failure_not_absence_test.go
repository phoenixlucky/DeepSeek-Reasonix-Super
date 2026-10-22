package history

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/historycatalog"
	"reasonix/internal/state/store"
)

// useInMemoryProcessCatalog keeps the shared catalog off the real cache and on
// this test's own data, the way the other history tests do.
func useInMemoryProcessCatalog(t *testing.T) {
	t.Helper()
	prevOpen := processHistoryCatalog.open
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = processHistoryCatalog.close(ctx)
		processHistoryCatalog = indexedCatalogManager{open: prevOpen}
	})
	processHistoryCatalog.open = func(ctx context.Context, opts historycatalog.Options) (*historycatalog.Catalog, error) {
		opts.Path, opts.InMemory = "", true
		return historycatalog.Open(ctx, opts)
	}
}

// requireDeniableReads skips a permission fixture on hosts where chmod cannot
// deny a read: Windows maps 000 to the read-only attribute, and root ignores
// the mode bits entirely.
func requireDeniableReads(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 cannot deny a read on this platform/user")
	}
}

func waitIndexed(t *testing.T, ctx context.Context, query string, want int) {
	t.Helper()
	for {
		if got, err := SearchSessions(ctx, query, 10); err == nil && len(got) == want {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("index never surfaced %d session(s) for %q", want, query)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitProjectionComplete waits until the catalog's own status says every root
// is scanned and every source is healthy, so a later "still building" notice
// can only come from the failure under test.
func waitProjectionComplete(t *testing.T, ctx context.Context) {
	t.Helper()
	for {
		status := processHistoryCatalog.get().Status()
		if status.State == "ready" && status.Pending == 0 && status.Total > 0 && status.Indexed == status.Total {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("projection never completed: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A transcript whose bytes cannot be read must not be reported as a query
// that matched nothing. The index still holds the row (health ok, digest
// asserted), the searcher loads the transcript to build a snippet, the load
// fails, and the candidate is dropped: no error, no partial notice, no
// reindex. The digest-mismatch branch above does enqueue a repair.
func TestUnreadableTranscriptIsNotReportedAsNoResults(t *testing.T) {
	requireDeniableReads(t)
	useInMemoryProcessCatalog(t)
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "unreadable.jsonl")
	saveSession(t, path,
		provider.Message{Role: provider.RoleUser, Content: "which zebra migration order did we choose?"},
		provider.Message{Role: provider.RoleAssistant, Content: "the phoenix-first zebra order"})

	tool := NewIndexedTool(Options{SessionDir: dir, GlobalSessionDir: dir})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitIndexed(t, ctx, "zebra migration", 1)
	waitProjectionComplete(t, ctx)

	// The transcript is now unreadable while its ledger identity still matches
	// the row: a permission fault on the event log, not an edit.
	logPath := store.SessionEventLog(path)
	if err := os.Chmod(logPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(logPath, 0o600) })

	args, err := json.Marshal(map[string]any{"operation": "search", "query": "zebra migration", "scope": "global"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Execute(ctx, args)
	if err != nil {
		return // reporting the read failure as an error satisfies the contract
	}
	if strings.HasPrefix(out, "No saved session history matched") {
		t.Fatalf("an indexed session became unreadable and the tool answered %q: "+
			"a read failure is being reported as a query that matched nothing", firstLine(out))
	}

	// The failure has to reach the projection's own status surface, not just
	// stall this one call: the source is corrupt and Indexed < Total.
	status := processHistoryCatalog.get().Status()
	if status.Failed == 0 || status.Indexed >= status.Total {
		t.Fatalf("unreadable transcript left the projection certifying itself complete: %+v", status)
	}
	notice := fmt.Sprintf("History index is still building (%d/%d); results may be incomplete.", status.Indexed, status.Total)
	if !strings.Contains(out, notice) {
		t.Fatalf("tool output does not carry the existing incomplete-projection notice %q:\n%s", notice, out)
	}
}

// Control: with every transcript readable, a query that genuinely matches
// nothing still answers "no matches" and says nothing about a broken index.
func TestUnmatchedQueryWithReadableTranscriptsStillSaysNoMatches(t *testing.T) {
	useInMemoryProcessCatalog(t)
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "readable.jsonl")
	saveSession(t, path,
		provider.Message{Role: provider.RoleUser, Content: "which zebra migration order did we choose?"},
		provider.Message{Role: provider.RoleAssistant, Content: "the phoenix-first zebra order"})

	tool := NewIndexedTool(Options{SessionDir: dir, GlobalSessionDir: dir})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitIndexed(t, ctx, "zebra migration", 1)
	waitProjectionComplete(t, ctx)

	args, err := json.Marshal(map[string]any{"operation": "search", "query": "quux nonexistent", "scope": "global"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if want := `No saved session history matched "quux nonexistent".`; !strings.HasPrefix(out, want) {
		t.Fatalf("a readable, complete projection answered a genuinely empty query without %q:\n%s", want, out)
	}
	if strings.Contains(out, "History index is still building") {
		t.Fatalf("a genuinely empty query raised a false incomplete-index alarm:\n%s", out)
	}
	if status := processHistoryCatalog.get().Status(); status.Failed != 0 || status.Indexed != status.Total {
		t.Fatalf("a genuinely empty query left the projection unhealthy: %+v", status)
	}
}

// Once the transcript can be read again, the periodic scan has to notice even
// though nothing about its size or mtime changed. This drives the scan that
// retries a non-ok source (#13923) end to end through the tool.
func TestUnreadableTranscriptServesContentAgainAfterTheScanRetries(t *testing.T) {
	requireDeniableReads(t)
	useInMemoryProcessCatalog(t)
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "healed.jsonl")
	saveSession(t, path,
		provider.Message{Role: provider.RoleUser, Content: "which zebra migration order did we choose?"},
		provider.Message{Role: provider.RoleAssistant, Content: "the phoenix-first zebra order"})

	tool := NewIndexedTool(Options{SessionDir: dir, GlobalSessionDir: dir})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitIndexed(t, ctx, "zebra migration", 1)
	waitProjectionComplete(t, ctx)
	catalog := processHistoryCatalog.get()
	root := historycatalog.Root{Path: dir, Source: "project", Scope: "project", WorkspaceRoot: dir}

	logPath := store.SessionEventLog(path)
	if err := os.Chmod(logPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(logPath, 0o600) })

	args, err := json.Marshal(map[string]any{"operation": "search", "query": "zebra migration", "scope": "global"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Execute(ctx, args); err != nil {
		t.Fatalf("search while unreadable: %v", err)
	}
	if status := catalog.Status(); status.Failed == 0 {
		t.Fatalf("unreadable transcript was not recorded as a failed source: %+v", status)
	}
	// Let the queued retry observe the same unreadable file, so the heal below
	// is answered by the scan rather than by an in-flight queue entry.
	if err := catalog.Flush(ctx); err != nil {
		t.Fatalf("flush while unreadable: %v", err)
	}
	if status := catalog.Status(); status.Indexed >= status.Total {
		t.Fatalf("projection certifies itself complete while a source is unreadable: %+v", status)
	}

	// The fault heals at the same size and mtime: only the ability to read
	// changed, which is the fact the projection got wrong.
	if err := os.Chmod(logPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReconcileRoot(ctx, root); err != nil {
		t.Fatalf("scan after the fault healed: %v", err)
	}
	waitIndexed(t, ctx, "zebra migration", 1)
	if status := catalog.Status(); status.Failed != 0 || status.Indexed != status.Total {
		t.Fatalf("after the fault healed the projection is still incomplete: %+v", status)
	}
	out, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("search after the fault healed: %v", err)
	}
	if !strings.Contains(out, "History search results") {
		t.Fatalf("search after the fault healed did not serve the recovered transcript:\n%s", out)
	}
	if strings.Contains(out, "History index is still building") {
		t.Fatalf("search after the fault healed still reports an incomplete index:\n%s", out)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
