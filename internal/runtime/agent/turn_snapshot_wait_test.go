package agent

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/state/observation"
)

// slowSnapshot is a snapshot that ends only when it is told to: it records that
// it was cancelled and answers with the incomplete snapshot a cancelled walk gives.
func slowSnapshot(t *testing.T, ctx context.Context) (*turnSnapshot, chan struct{}) {
	t.Helper()
	snapCtx, cancel := context.WithCancel(ctx)
	ts := &turnSnapshot{done: make(chan struct{}), cancel: cancel}
	cancelled := make(chan struct{})
	go func() {
		<-snapCtx.Done()
		close(cancelled)
		ts.snap = observation.Snapshot{Incomplete: observation.IncompleteCancelled}
		close(ts.done)
	}()
	return ts, cancelled
}

func TestFirstToolIsNotHeldPastTheBoundByAnUnfinishedSnapshot(t *testing.T) {
	ts, cancelled := slowSnapshot(t, context.Background())
	start := time.Now()
	snap, ok := ts.await(context.Background(), 40*time.Millisecond)
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("the first tool waited %v on a snapshot that was not finishing", waited)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("the unfinished snapshot was left running beside the tool it was meant to precede")
	}
	if !ok || snap.Complete || snap.Incomplete != observation.IncompleteCancelled {
		t.Fatalf("snapshot = %+v (ok %v), want the cancelled, incomplete one so the bundle records a gap", snap, ok)
	}
}

func TestAFinishedSnapshotIsHandedOverAtOnce(t *testing.T) {
	ts := &turnSnapshot{done: make(chan struct{}), cancel: func() { t.Error("a finished snapshot was cancelled") }}
	ts.snap = observation.Snapshot{Complete: true, Entries: 3}
	close(ts.done)
	if snap, ok := ts.await(context.Background(), time.Hour); !ok || snap.Entries != 3 {
		t.Fatalf("snapshot = %+v (ok %v)", snap, ok)
	}
}

func TestACancelledTurnStopsWaitingForTheSnapshot(t *testing.T) {
	ts, _ := slowSnapshot(t, context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := ts.await(ctx, time.Hour); ok {
		t.Fatal("a cancelled turn was handed a snapshot")
	}
	ts.cancel()
}

func TestAnUnstoppableSnapshotDoesNotHoldTheToolPastTheGrace(t *testing.T) {
	ts := &turnSnapshot{done: make(chan struct{}), cancel: func() {}}
	start := time.Now()
	if _, ok := ts.await(context.Background(), 20*time.Millisecond); ok {
		t.Fatal("a snapshot that never finished was reported as taken")
	}
	if waited := time.Since(start); waited > snapshotCancelGrace+2*time.Second {
		t.Fatalf("waited %v", waited)
	}
}
