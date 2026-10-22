package acp

import (
	"strings"
	"testing"

	"reasonix/internal/contract/event"
)

func chunkID(t *testing.T, fn *fakeNotifier, i int) string {
	t.Helper()
	id, _ := fn.updateMap(t, i)["messageId"].(string)
	return id
}

// Chunks of one sampling attempt share an id and a retried attempt is a new
// message, so a client never glues a discarded draft to the answer.
func TestMessageChunksOfOneAttemptShareAnIdAndARetryChangesIt(t *testing.T) {
	fn := &fakeNotifier{}
	sink := newUpdateSink(fn, "sess-1")
	begin := func(id string) {
		sink.Emit(event.Event{Kind: event.StreamAttempt, StreamAttempt: event.StreamAttemptInfo{ID: id, Action: event.StreamAttemptBegin}})
	}
	end := func(id string, a event.StreamAttemptAction) {
		sink.Emit(event.Event{Kind: event.StreamAttempt, StreamAttempt: event.StreamAttemptInfo{ID: id, Action: a}})
	}
	begin("sa-1-1")
	sink.Emit(event.Event{Kind: event.Text, Text: "draft "})
	sink.Emit(event.Event{Kind: event.Text, Text: "text"})
	end("sa-1-1", event.StreamAttemptDiscard)
	begin("sa-2-2")
	sink.Emit(event.Event{Kind: event.Text, Text: "answer"})
	end("sa-2-2", event.StreamAttemptCommit)
	a, b, c := chunkID(t, fn, 0), chunkID(t, fn, 1), chunkID(t, fn, 2)
	if a == "" || a != b {
		t.Fatalf("chunks of one attempt: %q vs %q", a, b)
	}
	if c == "" || c == a {
		t.Fatalf("a retried attempt kept the id: %q vs %q", c, a)
	}
}

func TestAChunkOutsideAnyAttemptCarriesNoIdRatherThanAWrongOne(t *testing.T) {
	fn := &fakeNotifier{}
	newUpdateSink(fn, "sess-1").Emit(event.Event{Kind: event.Text, Text: "synthetic"})
	if id := chunkID(t, fn, 0); id != "" {
		t.Fatalf("id = %q", id)
	}
}

func TestMessageIdsDoNotCollideAcrossSessionOpens(t *testing.T) {
	ids := map[string]bool{}
	for range 20 {
		fn := &fakeNotifier{}
		sink := newUpdateSink(fn, "sess-1")
		sink.Emit(event.Event{Kind: event.StreamAttempt, StreamAttempt: event.StreamAttemptInfo{ID: "sa-1-1", Action: event.StreamAttemptBegin}})
		sink.Emit(event.Event{Kind: event.Text, Text: "x"})
		id := chunkID(t, fn, 0)
		if ids[id] || !strings.HasSuffix(id, "-sa-1-1") {
			t.Fatalf("id %q repeated or malformed", id)
		}
		ids[id] = true
	}
}
