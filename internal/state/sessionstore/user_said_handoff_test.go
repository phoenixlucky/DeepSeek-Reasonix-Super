package sessionstore

import (
	"testing"

	"reasonix/internal/contract/provider"
)

// A legacy executor-handoff turn wraps the person's task in a host envelope. The
// index and every display reader must cut it at the task, or a query on "planner"
// would match words the person never wrote.
func TestUserSaidTextUnwrapsALegacyHandoffUpFront(t *testing.T) {
	handoff := "# " + ExecutorHandoffMarker + "\n\nOriginal task:\nfix the flaky test\n\nPlanner output:\nfirst do A, then B\n"
	want := "fix the flaky test"
	if got := UserSaidText(handoff); got != want {
		t.Fatalf("UserSaidText(handoff) = %q, want %q", got, want)
	}
	msg := provider.Message{Role: provider.RoleUser, Content: handoff}
	if got := UserMessageText(msg); got != want {
		t.Fatalf("the recall/display reader disagrees with the index on a handoff turn: %q, want %q", got, want)
	}
	if got := UserMessageText(provider.Message{Role: provider.RoleUser, RawContent: handoff}); got != want {
		t.Fatalf("the RawContent branch disagrees on a handoff turn: %q, want %q", got, want)
	}
}
