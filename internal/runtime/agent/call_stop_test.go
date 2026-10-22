package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent/testutil"
	"reasonix/internal/state/sessionstore"
)

func TestStoppingOneCallLetsTheTurnReachTheModelWithTheCause(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(slowTool{})
	mp := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "c1", Name: "slow_tool", Arguments: `{"duration_ms": 10000}`}}},
		testutil.Turn{Text: "moved on"},
	)
	a := New(mp, reg, sessionstore.NewSession(""), Options{}, &recordSink{})

	done := make(chan error, 1)
	go func() { done <- a.Run(context.Background(), "run it") }()

	deadline := time.Now().Add(2 * time.Second)
	for !a.CancelToolCall("c1") {
		if time.Now().After(deadline) {
			t.Fatal("call c1 never became stoppable")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stopping one call must not fail the turn, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("turn did not continue after the call was stopped")
	}
	reqs := mp.Requests()
	if len(reqs) != 2 {
		t.Fatalf("model requests = %d, want 2 (the turn goes on)", len(reqs))
	}
	if got := requestContents(reqs[1]); !strings.Contains(got, tool.CodeCancelledByUser) {
		t.Fatalf("the model was not told why the call ended: %s", got)
	}
	if a.CancelToolCall("c1") {
		t.Fatal("a finished call must not be stoppable")
	}
}

func TestStoppingAnUnknownCallDoesNothing(t *testing.T) {
	a := New(testutil.NewMock("m"), tool.NewRegistry(), sessionstore.NewSession(""), Options{}, &recordSink{})
	if a.CancelToolCall("nope") {
		t.Fatal("unknown call id reported as stopped")
	}
}
