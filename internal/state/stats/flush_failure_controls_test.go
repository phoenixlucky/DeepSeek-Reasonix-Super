package stats

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

func readDailyRecords(t *testing.T, dir string) []record {
	t.Helper()
	files := dailyJSONLFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("want exactly 1 daily file in %s, got %d", dir, len(files))
	}
	f, err := os.Open(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("open daily file: %v", err)
	}
	defer f.Close()
	records, err := decodeRecords(f)
	if err != nil {
		t.Fatalf("decode daily file: %v", err)
	}
	return records
}

// TestFlushHealthyPersistsRecords is the audit test's control: with a writable
// stats dir, Flush must still return nil, and the records it accepted must be
// readable back off disk.
func TestFlushHealthyPersistsRecords(t *testing.T) {
	dir := testenv.TempDir(t)
	r := NewRecorder(&spySink{}, dir, "desktop")
	r.Emit(usageEvent("deepseek/deepseek-v4-pro", 100, 50, 0, 0, 0, 150))
	r.Emit(turnEvent())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Flush(ctx); err != nil {
		t.Fatalf("healthy Flush returned error: %v", err)
	}

	records := readDailyRecords(t, dir)
	if len(records) != 2 {
		t.Fatalf("want 2 records on disk after a healthy flush, got %d", len(records))
	}
	if !records[1].Turn {
		t.Fatalf("turn marker was not persisted: %+v", records)
	}
}

// TestFlushReportsEachFailureThenConverges pins the two halves of the contract:
// a flush reports only the appends it ordered (a still-broken dir keeps
// failing), and the reported failure is consumed by the flush that reported it,
// so a later successful flush returns nil instead of staying poisoned.
func TestFlushReportsEachFailureThenConverges(t *testing.T) {
	parent := testenv.TempDir(t)
	notADir := filepath.Join(parent, "stats-is-a-file")
	if err := os.WriteFile(notADir, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	r := NewRecorder(&spySink{}, notADir, "desktop")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r.Emit(turnEvent())
	if err := r.Flush(ctx); err == nil {
		t.Fatalf("first Flush did not report the failed append")
	}

	r.Emit(turnEvent())
	if err := r.Flush(ctx); err == nil {
		t.Fatalf("Flush over a still-broken dir returned nil instead of re-reporting")
	}

	if err := os.Remove(notADir); err != nil {
		t.Fatalf("remove blocker: %v", err)
	}
	r.Emit(turnEvent())
	if err := r.Flush(ctx); err != nil {
		t.Fatalf("Flush after recovery re-reported an already-reported failure: %v", err)
	}
	records := readDailyRecords(t, notADir)
	if len(records) != 1 {
		t.Fatalf("want the one record written after recovery, got %d", len(records))
	}
}
