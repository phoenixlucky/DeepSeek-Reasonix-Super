package acp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

type cancelRig struct {
	client  *rpcClient
	started chan string
}

func newCancelRig(t *testing.T) *cancelRig {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	conn := NewConn(inR, outW)
	rig := &cancelRig{started: make(chan string, 8)}
	conn.Handle("slow", func(ctx context.Context, _ json.RawMessage) (any, error) {
		rig.started <- "slow"
		<-ctx.Done()
		return nil, ctx.Err()
	})
	conn.Handle("partial", func(ctx context.Context, _ json.RawMessage) (any, error) {
		rig.started <- "partial"
		<-ctx.Done()
		return map[string]string{"stopReason": "cancelled"}, nil
	})
	conn.Handle("fast", func(context.Context, json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })
	done := make(chan struct{})
	go func() { _ = conn.Serve(context.Background()); close(done) }()
	rig.client = newRPCClient(inW, outR)
	t.Cleanup(func() { _ = inW.Close(); <-done; _ = outW.Close() })
	return rig
}

func (r *cancelRig) awaitStart(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.started:
		if got != want {
			t.Fatalf("started %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never started", want)
	}
}

func (r *cancelRig) answer(t *testing.T, ch chan frame) frame {
	t.Helper()
	select {
	case f := <-ch:
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
		return frame{}
	}
}

func TestCancelRequestEndsOnlyThatRequestAndAnswersCancelled(t *testing.T) {
	r := newCancelRig(t)
	first := r.client.callAsync("slow", map[string]any{})
	r.awaitStart(t, "slow")
	second := r.client.callAsync("slow", map[string]any{})
	r.awaitStart(t, "slow")
	r.client.notify("$/cancel_request", map[string]any{"requestId": 1})
	got := r.answer(t, first)
	if got.Error == nil || got.Error.Code != ErrRequestCancelled {
		t.Fatalf("cancelled request answered %+v, want -32800", got.Error)
	}
	select {
	case f := <-second:
		t.Fatalf("an unrelated request was answered: %+v", f)
	case <-time.After(150 * time.Millisecond):
	}
	r.client.notify("$/cancel_request", map[string]any{"requestId": 2})
	if got := r.answer(t, second); got.Error == nil || got.Error.Code != ErrRequestCancelled {
		t.Fatalf("second = %+v", got.Error)
	}
}

// A handler may answer a cancelled request with what it has: that is a valid
// response, and the code is for the one that had nothing to say.
func TestCancelRequestMayStillYieldAPartialResult(t *testing.T) {
	r := newCancelRig(t)
	ch := r.client.callAsync("partial", map[string]any{})
	r.awaitStart(t, "partial")
	r.client.notify("$/cancel_request", map[string]any{"requestId": 1})
	if got := r.answer(t, ch); got.Error != nil || len(got.Result) == 0 {
		t.Fatalf("partial result lost: %+v", got)
	}
}

func TestCancelRequestIgnoresWhatItCannotMatch(t *testing.T) {
	r := newCancelRig(t)
	r.client.notify("$/cancel_request", map[string]any{"requestId": 99})
	r.client.notify("$/cancel_request", "not an object")
	r.client.notify("$/cancel_request", map[string]any{})
	if got := r.answer(t, r.client.callAsync("fast", map[string]any{})); got.Error != nil {
		t.Fatalf("a stray cancel broke the connection: %+v", got.Error)
	}
	r.client.notify("$/cancel_request", map[string]any{"requestId": 4})
	r.client.notify("$/cancel_request", map[string]any{"requestId": "2"})
	if got := r.answer(t, r.client.callAsync("fast", map[string]any{})); got.Error != nil {
		t.Fatalf("cancelling an answered or differently typed id changed a later request: %+v", got.Error)
	}
}
