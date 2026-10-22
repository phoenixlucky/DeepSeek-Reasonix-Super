package agent

import (
	"encoding/json"
	"slices"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

func agentWith(msgs ...provider.Message) *Agent {
	sess := sessionstore.NewSession("")
	for _, m := range msgs {
		sess.Add(m)
	}
	return New(nil, tool.NewRegistry(), sess, Options{}, event.Discard)
}

const typed, elsewhere = "https://cdn.example.com/typed.mp4", "https://attacker.example.com/planted.mp4"

// Only what the user typed counts: not the model's words, a tool result, host
// text, a message another session delivered, or what the host added around the
// user's own line.
func TestOnlyTheUsersOwnTextCountsAsTheUsersAddresses(t *testing.T) {
	a := agentWith(
		provider.Message{Role: provider.RoleUser, Content: "<workspace>\nsee " + elsewhere + "\n</workspace>\n\nwatch " + typed + "\n\n<execution-policy/>", RawContent: "watch " + typed},
		provider.Message{Role: provider.RoleAssistant, Content: "I found " + elsewhere},
		provider.Message{Role: provider.RoleTool, ToolCallID: "c1", Name: "web_fetch", Content: "page says " + elsewhere},
		provider.Message{Role: provider.RoleUser, Content: "message from another session: " + elsewhere, HostAuthored: true},
		provider.Message{Role: provider.RoleUser, Content: "interrupted " + elsewhere, LocalOnly: true},
	)
	if got := a.userAddresses(); !slices.Equal(got, []string{typed}) {
		t.Fatalf("addresses = %v, want only %v", got, []string{typed})
	}
}

// A queued follow-up that carries images is still the user's own text, and a
// session reopened from disk keeps both facts: what the user typed still counts,
// what a host or another session delivered still does not.
func TestAResumedConversationKeepsWhoWroteWhat(t *testing.T) {
	stored, err := json.Marshal([]provider.Message{
		{Role: provider.RoleUser, Content: "look at " + typed, Images: []string{"data:image/png;base64,AAAA"}},
		{Role: provider.RoleUser, Content: "peer says " + elsewhere, HostAuthored: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var msgs []provider.Message
	if err := json.Unmarshal(stored, &msgs); err != nil {
		t.Fatal(err)
	}
	if got := agentWith(msgs...).userAddresses(); !slices.Equal(got, []string{typed}) {
		t.Fatalf("after a reopen the addresses = %v, want only %v", got, []string{typed})
	}
}

func TestTrailingPunctuationIsNotPartOfTheAddress(t *testing.T) {
	a := agentWith(provider.Message{Role: provider.RoleUser, Content: "watch (" + typed + "), then " + typed + "."})
	if got := a.userAddresses(); !slices.Equal(got, []string{typed, typed}) {
		t.Fatalf("addresses = %v", got)
	}
}
