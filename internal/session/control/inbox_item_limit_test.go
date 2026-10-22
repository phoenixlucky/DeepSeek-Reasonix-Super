package control

import (
	"errors"
	"fmt"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/state/sessioninbox"
)

func TestQueueHoldsMoreThanTheOldCountWhenTheUserSetsNoLimit(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	c, _, _ := newInboxDispatchController(t)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	for i := range sessioninbox.DefaultMaxItems + 5 {
		if _, err := c.EnqueueInbox(InboxRequest{Submit: fmt.Sprintf("line %d", i)}); err != nil {
			t.Fatalf("queued line %d refused with no limit set: %v", i, err)
		}
	}
}

func TestQueueStopsAtTheLimitTheUserSet(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	cfg := config.LoadForEdit(config.UserConfigPath())
	cfg.Agent.MaxQueuedInputs = 2
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newInboxDispatchController(t)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if _, err := c.EnqueueInbox(InboxRequest{Submit: fmt.Sprintf("line %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.EnqueueInbox(InboxRequest{Submit: "one too many"}); !errors.Is(err, sessioninbox.ErrCapacityItems) {
		t.Fatalf("third line = %v, want ErrCapacityItems", err)
	}
}
