package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A client that sees TurnDone asks for status at once. The window in which that
// event is still being delivered keeps admission closed, but the turn it
// reports is over, so what a window shows as "running" must say so while the
// guards keep refusing.
func TestTurnInFlightEndsWithTheTurnWhileAdmissionStaysClosedUntilDelivered(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	c := New(Options{Sink: holdFinishingWindow(release, entered, nil)})

	c.runGuarded(func(context.Context) error {
		if !c.TurnInFlight() {
			t.Error("a turn that is executing must read as in flight")
		}
		return nil
	})
	<-entered // TurnDone is being delivered; the finishing window is open

	if c.TurnInFlight() {
		t.Fatal("TurnInFlight is true while only the TurnDone delivery remains")
	}
	if !c.Running() {
		t.Fatal("Running must stay true through the delivery window: it is what admission guards read")
	}
	if err := c.RunTurn(context.Background(), "inside the window"); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("a turn started inside the window = %v, want ErrTurnRunning", err)
	}
	close(release)
	waitIdleAdmission(t, c)
	if c.TurnInFlight() {
		t.Fatal("TurnInFlight is true after the controller went idle")
	}
}

func TestTurnInFlightIsRaceFreeAgainstTheTurnEnding(t *testing.T) {
	c := New(Options{})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = c.TurnInFlight()
				_ = c.Running()
			}
		}
	}()
	for range 50 {
		c.runGuarded(func(context.Context) error { return nil })
		waitIdleAdmission(t, c)
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not stop")
	}
}
