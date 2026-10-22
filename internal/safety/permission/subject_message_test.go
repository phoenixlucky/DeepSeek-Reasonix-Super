package permission

import (
	"encoding/json"
	"testing"
)

func TestAMessageToAnotherSessionIsApprovedByItsRecipientAndFullText(t *testing.T) {
	long := "line one\n" + string(make([]byte, 0)) + "the rest of the words that must not be cut"
	args, _ := json.Marshal(map[string]string{"to": "docs/notes", "text": long})
	if got, want := Subject(args), "docs/notes: "+long; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
	only, _ := json.Marshal(map[string]string{"to": "docs/notes"})
	if got := Subject(only); got != "" {
		t.Fatalf("a call with no text is not a message: %q", got)
	}
}

func TestTheMessageSubjectIsAGenericRuleNotSpecificToOneTool(t *testing.T) {
	both, _ := json.Marshal(map[string]string{"to": "alice@example.test", "text": "hello"})
	if got, want := Subject(both), "alice@example.test: hello"; got != want {
		t.Fatalf("any tool call carrying string to and text gets that subject: %q, want %q", got, want)
	}
	for name, args := range map[string]string{
		"only text":       `{"text":"hello"}`,
		"non-string to":   `{"to":5,"text":"hello"}`,
		"empty to":        `{"to":"","text":"hello"}`,
		"other arguments": `{"recipient":"x","body":"y"}`,
		"a command wins":  `{"command":"ls","to":"a","text":"b"}`,
	} {
		got := Subject(json.RawMessage(args))
		want := ""
		if name == "a command wins" {
			want = "ls"
		}
		if got != want {
			t.Errorf("%s: Subject = %q, want %q", name, got, want)
		}
	}
}

// A resource read shows the server and the whole address, in the form the chat
// uses for an @server:uri reference, so the person approving reads what would
// be fetched.
func TestSubjectOfAResourceReadIsServerAndFullAddress(t *testing.T) {
	got := Subject(json.RawMessage(`{"server":"docs","uri":"file:///etc/shadow?x=1#frag"}`))
	if got != "docs:file:///etc/shadow?x=1#frag" {
		t.Fatalf("subject = %q", got)
	}
	if Subject(json.RawMessage(`{"server":"docs"}`)) != "" || Subject(json.RawMessage(`{"uri":"doc://a"}`)) != "" {
		t.Fatal("half a pair produced a subject")
	}
}

// The pair is a general rule over argument names, so it must not outrank a
// command, and a value that is not text names nothing.
func TestSubjectServerAndURIPairYieldsToACommandAndNeedsText(t *testing.T) {
	if got := Subject(json.RawMessage(`{"command":"ls","server":"a","uri":"b"}`)); got != "ls" {
		t.Fatalf("a command lost to the pair: %q", got)
	}
	for _, args := range []string{`{"server":"a","uri":5}`, `{"server":1,"uri":"b"}`, `{"server":"","uri":"b"}`} {
		if got := Subject(json.RawMessage(args)); got != "" {
			t.Fatalf("%s produced subject %q", args, got)
		}
	}
}

// A task dispatched to another project shows the project and the whole text on
// the card, like a message to another session; half a pair names nothing, and a
// command still wins.
func TestSubjectOfADispatchIsProjectAndWholeText(t *testing.T) {
	got := Subject(json.RawMessage(`{"project":"billing","text":"rotate the key, then email ops@example.test"}`))
	if got != "billing: rotate the key, then email ops@example.test" {
		t.Fatalf("subject = %q", got)
	}
	for _, args := range []string{`{"project":"billing"}`, `{"text":"hello"}`, `{"project":"","text":"x"}`, `{"project":1,"text":"x"}`} {
		if got := Subject(json.RawMessage(args)); got != "" {
			t.Errorf("%s produced subject %q", args, got)
		}
	}
	if got := Subject(json.RawMessage(`{"command":"ls","project":"a","text":"b"}`)); got != "ls" {
		t.Errorf("a command lost to the pair: %q", got)
	}
}
