package tui

import (
	"strings"
	"testing"

	"reasonix/internal/contract/eventwire"
)

func TestAskCountdownIsShownAndClearedWhenTheQuestionIsTakenOver(t *testing.T) {
	m, _ := testModel(t)
	ev := askEvent()
	ev.Ask.SkipInMs = 90_000
	apply(m, ev)
	open := m.tr.OpenPrompt()
	if open == nil || open.AskSkipAt.IsZero() {
		t.Fatalf("an announced skip left no deadline on the card: %+v", open)
	}
	panel := strings.Join(m.askPanel(open), "\n")
	if !strings.Contains(panel, "1:") && !strings.Contains(panel, "89s") {
		t.Fatalf("the panel shows no countdown:\n%s", panel)
	}

	apply(m, eventwire.Event{Kind: "ask_engaged", Ask: &eventwire.Ask{ID: "ask1"}})
	if got := len(m.tr.Items); got != 1 {
		t.Fatalf("taking the question over drew %d cards, want 1", got)
	}
	if !m.tr.OpenPrompt().AskSkipAt.IsZero() {
		t.Fatal("taking the question over left the deadline on the card")
	}
	if strings.Contains(strings.Join(m.askPanel(m.tr.OpenPrompt()), "\n"), "skipped in") {
		t.Fatal("a question that is not skipped still shows a countdown")
	}
}

func TestAskWithoutASkipShowsNoCountdown(t *testing.T) {
	m, _ := testModel(t)
	apply(m, askEvent())
	if panel := strings.Join(m.askPanel(m.tr.OpenPrompt()), "\n"); strings.Contains(panel, "skipped in") {
		t.Fatalf("countdown on a question that is never skipped:\n%s", panel)
	}
}
