package testutil

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
)

type cancellationObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *cancellationObservedContext) Err() error {
	err := c.Context.Err()
	if err != nil {
		c.once.Do(func() { close(c.observed) })
	}
	return err
}

func TestMockProviderStreamHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mp := NewMock("mock", Turn{Text: "hello"})
	ch, err := mp.Stream(ctx, provider.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream error = %v, want context.Canceled", err)
	}
	if ch != nil {
		t.Fatal("Stream returned a channel for canceled context")
	}
	if got := mp.CallCount(); got != 0 {
		t.Fatalf("CallCount = %d, want 0", got)
	}
}

func TestMockProviderStreamStopsOnContextCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancellationObservedContext{Context: parent, observed: make(chan struct{})}
	mp := NewMock("mock", Turn{
		Text: "first",
		ToolCalls: []provider.ToolCall{
			{ID: "call-1", Name: "noop", Arguments: `{}`},
		},
	})

	ch, err := mp.Stream(ctx, provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := (<-ch).Text; got != "first" {
		t.Fatalf("first chunk text = %q, want first", got)
	}
	cancel()
	// Cancellation and a pending send can both be selectable. Wait until the
	// producer observes cancellation before requiring its next error chunk.
	select {
	case <-ctx.observed:
	case <-time.After(testenv.Budget(t)):
		t.Fatal("producer did not observe context cancellation")
	}
	chunk, ok := <-ch
	if !ok {
		t.Fatal("stream closed without returning cancellation error")
	}
	if chunk.Type != provider.ChunkError || !errors.Is(chunk.Err, context.Canceled) {
		t.Fatalf("chunk after cancellation = %#v, want context.Canceled error", chunk)
	}
	if _, ok := <-ch; ok {
		t.Fatal("stream stayed open after cancellation error")
	}
}
