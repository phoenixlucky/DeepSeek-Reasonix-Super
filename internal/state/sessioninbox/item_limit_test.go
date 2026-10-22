package sessioninbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestNoItemLimitQueuesPastTheDefaultCount(t *testing.T) {
	dir := testenv.TempDir(t)
	session := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(session, []byte("{}\n"), 0o644)
	s, err := Open(session, Limits{MaxItems: NoItemLimit})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := range DefaultMaxItems + 10 {
		if _, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: fmt.Sprintf("item %d", i)}}); err != nil {
			t.Fatalf("item %d refused with no item limit: %v", i, err)
		}
	}
	if got := s.Snapshot().Capacity.MaxItems; got != 0 {
		t.Fatalf("capacity reports a count limit of %d for an unlimited queue", got)
	}
}

func TestNoItemLimitStillBoundsBytes(t *testing.T) {
	dir := testenv.TempDir(t)
	session := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(session, []byte("{}\n"), 0o644)
	s, err := Open(session, Limits{MaxItems: NoItemLimit, MaxTotalBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var last error
	for i := range 50 {
		_, last = s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: fmt.Sprintf("padding line number %d", i)}})
		if last != nil {
			break
		}
	}
	if !errors.Is(last, ErrCapacityBytes) {
		t.Fatalf("an unlimited-count queue must still refuse at its byte limit: %v", last)
	}
}
