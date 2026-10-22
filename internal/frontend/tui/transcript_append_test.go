package tui

import (
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/contract/eventwire"
)

// allocated reports the bytes the run allocated in total, which is what a
// string that is copied whole on every append costs and a growing buffer does
// not: it does not depend on how fast the machine is.
func allocated(run func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	run()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

const (
	appendEvents = 4000
	appendChunk  = 200
	// What 4000 chunks of 200 bytes cost when each append copies everything so
	// far is about 1.6 GB; a growing buffer needs a few MB.
	appendBudget = 64 << 20
)

func TestStreamedAnswerGrowsWithoutCopyingItselfEachDelta(t *testing.T) {
	var tr Transcript
	delta := strings.Repeat("a", appendChunk)
	if got := allocated(func() {
		for range appendEvents {
			tr.Apply(eventwire.Event{Kind: "text", Text: delta})
		}
	}); got > appendBudget {
		t.Fatalf("%d answer deltas allocated %d MB; appending must not copy the answer each time", appendEvents, got>>20)
	}
	if want := appendEvents * appendChunk; len(tr.Items[0].Text) != want {
		t.Fatalf("answer length = %d, want %d", len(tr.Items[0].Text), want)
	}
}

func TestStreamedThinkingGrowsWithoutCopyingItselfEachDelta(t *testing.T) {
	var tr Transcript
	delta := strings.Repeat("b", appendChunk)
	if got := allocated(func() {
		for range appendEvents {
			tr.Apply(eventwire.Event{Kind: "reasoning", Text: delta})
		}
	}); got > appendBudget {
		t.Fatalf("%d thinking deltas allocated %d MB; appending must not copy the thinking each time", appendEvents, got>>20)
	}
	if want := appendEvents * appendChunk; len(tr.Items[0].Reasoning) != want {
		t.Fatalf("thinking length = %d, want %d", len(tr.Items[0].Reasoning), want)
	}
}

func TestStreamedToolOutputGrowsWithoutCopyingItselfEachEvent(t *testing.T) {
	var tr Transcript
	tr.Apply(eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "c1", Name: "bash"}})
	chunk := strings.Repeat("c", appendChunk)
	if got := allocated(func() {
		for range appendEvents {
			tr.Apply(eventwire.Event{Kind: "tool_progress", Tool: &eventwire.Tool{ID: "c1", Name: "bash", Output: chunk}})
		}
	}); got > appendBudget {
		t.Fatalf("%d output events allocated %d MB; appending must not copy the output each time", appendEvents, got>>20)
	}
	if want := appendEvents * appendChunk; len(tr.Items[0].Streamed) != want {
		t.Fatalf("streamed length = %d, want %d", len(tr.Items[0].Streamed), want)
	}
}

// A frame that replaces what was streamed (the kernel's record wins over the
// deltas) must not be lost to a buffer still holding the old text.
func TestAppendAfterTheTextWasReplacedStartsFromTheReplacement(t *testing.T) {
	var tr Transcript
	tr.Apply(eventwire.Event{Kind: "text", Text: "hello "})
	tr.Items[0].Text = "HELLO "
	tr.Apply(eventwire.Event{Kind: "text", Text: "world"})
	if got := tr.Items[0].Text; got != "HELLO world" {
		t.Fatalf("text = %q, want the replacement followed by the new delta", got)
	}
}
