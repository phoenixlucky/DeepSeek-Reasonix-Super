package jobs

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/event"
)

// TestManagerConcurrentAccess hammers every public Manager method from many
// goroutines. Any map access that escapes m.mu trips the runtime's built-in
// "concurrent map writes/read" fatal even without the race detector.
func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()

	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		go func(w int) {
			defer wg.Done()
			for i := range 200 {
				switch (w + i) % 6 {
				case 0:
					j := m.Start("bash", "x", func(ctx context.Context, out io.Writer) (string, error) {
						_, _ = out.Write([]byte("tick"))
						return "done", nil
					})
					_, _, _ = m.Output(j.ID)
				case 1:
					_ = m.Running()
				case 2:
					_ = m.DrainCompletedNote()
				case 3:
					_, _ = m.Wait(context.Background(), nil, WaitOptions{Timeout: time.Millisecond}) // returns a running snapshot
				case 4:
					m.Kill("bash-1")
				case 5:
					_, _, _ = m.Output("bash-2")
				}
			}
		}(w)
	}
	wg.Wait()
}

// A job started while the manager closes must either join the wait group before
// Close waits on it or be refused: Add racing Wait at a zero count is the
// misuse sync.WaitGroup documents, and the race detector reports it.
func TestStartRacingCloseNeverAddsToAWaitInProgress(t *testing.T) {
	for range 50 {
		m := NewManager(event.Discard)
		var wg sync.WaitGroup
		wg.Add(2)
		started := make(chan *Job, 64)
		go func() {
			defer wg.Done()
			for range 64 {
				started <- m.Start("bash", "x", func(context.Context, io.Writer) (string, error) { return "", nil })
			}
			close(started)
		}()
		go func() {
			defer wg.Done()
			m.CloseWithGrace(time.Second)
		}()
		wg.Wait()
		for j := range started {
			select {
			case <-j.done:
			case <-time.After(5 * time.Second):
				t.Fatalf("job %s never finished after close", j.ID)
			}
		}
	}
}

// A job refused because the manager already closed still ends: it is
// Interrupted, and the task recorder that saw it start sees it finish.
func TestStartAfterCloseEndsTheJobItRecorded(t *testing.T) {
	rec := &recordingRecorder{}
	m := newManagerWithRecorder(rec)
	m.CloseWithGrace(time.Second)
	ran := false
	j := m.Start("bash", "late", func(context.Context, io.Writer) (string, error) { ran = true; return "", nil })
	<-j.done
	j.mu.Lock()
	status := j.status
	j.mu.Unlock()
	if ran || status != Interrupted {
		t.Fatalf("late job ran=%v status=%v", ran, status)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.starts) != 1 || len(rec.dones) != 1 {
		t.Fatalf("recorder saw %d starts and %d ends", len(rec.starts), len(rec.dones))
	}
}
