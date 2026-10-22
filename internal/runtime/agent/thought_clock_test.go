package agent

import (
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

func TestThoughtClockRunsFromFirstReasoningToFirstAnswer(t *testing.T) {
	t0 := time.Unix(100, 0)
	var c thoughtClock
	c.answer(t0) // an answer before any reasoning is not a thought
	if c.ms() != 0 {
		t.Fatalf("no reasoning measured %d ms", c.ms())
	}
	c.reasoning(t0.Add(time.Second))
	c.reasoning(t0.Add(2 * time.Second))
	if got := c.ms(); got != 1000 {
		t.Fatalf("mid-thought = %d ms, want 1000 (to the last reasoning)", got)
	}
	c.answer(t0.Add(3500 * time.Millisecond))
	c.reasoning(t0.Add(9 * time.Second)) // reasoning after the answer began does not extend it
	c.answer(t0.Add(10 * time.Second))
	if got := c.ms(); got != 2500 {
		t.Fatalf("thought = %d ms, want 2500", got)
	}
}

func TestThoughtClockFirstTokenIsTheFirstOutputOfAnyKind(t *testing.T) {
	t0 := time.Unix(100, 0)
	c := thoughtClock{begin: t0}
	if c.firstMs() != 0 {
		t.Fatalf("nothing arrived yet, first token = %d ms", c.firstMs())
	}
	c.observe(provider.Chunk{Type: provider.ChunkReasoning}, t0.Add(100*time.Millisecond)) // metadata only
	if c.firstMs() != 0 {
		t.Fatalf("an empty reasoning chunk counted as the first token: %d ms", c.firstMs())
	}
	c.observe(provider.Chunk{Type: provider.ChunkReasoning, Text: "hm"}, t0.Add(700*time.Millisecond))
	c.observe(provider.Chunk{Type: provider.ChunkText, Text: "hi"}, t0.Add(900*time.Millisecond))
	if got := c.firstMs(); got != 700 {
		t.Fatalf("first token = %d ms, want 700", got)
	}

	answered := thoughtClock{begin: t0}
	answered.observe(provider.Chunk{Type: provider.ChunkToolCallStart}, t0.Add(300*time.Millisecond))
	if got := answered.firstMs(); got != 300 {
		t.Fatalf("a tool call is the first output too: %d ms, want 300", got)
	}
	if (&thoughtClock{}).firstMs() != 0 {
		t.Fatal("a clock that never began has no first token")
	}
}
