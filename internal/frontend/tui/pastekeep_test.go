package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func longPaste(tag string) string {
	var b strings.Builder
	for i := range 8 {
		b.WriteString(tag + "-line-" + strconv.Itoa(i) + "\n")
	}
	return b.String()
}

func attachmentCalls(k *recordingKernel) (n int) {
	for _, c := range k.seen() {
		if strings.HasPrefix(c, "POST /attachments") {
			n++
		}
	}
	return n
}

// Keeping a folded paste swaps the text for the pointer to the saved file, so
// the submit carries the pointer and none of the pasted lines.
func TestKeepingAFoldedPasteSendsOnlyThePointer(t *testing.T) {
	m, k := testModel(t)
	typeText(m, "explain ")
	m.Update(tea.PasteMsg{Content: longPaste("ALPHA")})
	run(m, press(m, "ctrl+g"))
	if got := m.composer.Value(); !strings.Contains(got, "[Pasted file #1 · 9 lines]") || strings.Contains(got, "[Pasted text") {
		t.Fatalf("composer = %q", got)
	}
	run(m, press(m, "enter"))
	var submit string
	for _, c := range k.seen() {
		if strings.HasPrefix(c, "POST /submit") {
			submit = c
		}
	}
	if !strings.Contains(submit, "Saved: .reasonix/attachments/p.txt") || strings.Contains(submit, "ALPHA-line-3") {
		t.Fatalf("submit = %s", submit)
	}
}

// Nothing moves unless asked: a folded paste is sent whole, as before.
func TestAFoldedPasteIsSentWholeUnlessKept(t *testing.T) {
	m, k := testModel(t)
	m.Update(tea.PasteMsg{Content: longPaste("BETA")})
	run(m, press(m, "enter"))
	if attachmentCalls(k) != 0 || !strings.Contains(strings.Join(k.seen(), "\n"), "BETA-line-3") {
		t.Fatalf("calls = %q", k.seen())
	}
}

func TestKeepingWithoutAFoldedPasteAsksNothing(t *testing.T) {
	m, k := testModel(t)
	typeText(m, "plain text")
	run(m, press(m, "ctrl+g"))
	if attachmentCalls(k) != 0 || m.composer.Value() != "plain text" {
		t.Fatalf("composer %q, calls %q", m.composer.Value(), k.seen())
	}
}

func TestKeepingTakesEveryFoldedPasteInTheDraftOnce(t *testing.T) {
	m, k := testModel(t)
	m.Update(tea.PasteMsg{Content: longPaste("ONE")})
	m.Update(tea.PasteMsg{Content: longPaste("TWO")})
	run(m, press(m, "ctrl+g"))
	run(m, press(m, "ctrl+g"))
	if n := attachmentCalls(k); n != 2 {
		t.Fatalf("attachment calls = %d, want one per paste", n)
	}
	if got := m.composer.Value(); strings.Count(got, "[Pasted file #") != 2 {
		t.Fatalf("composer = %q", got)
	}
}

// A refused save leaves the paste folded and sendable as it was.
func TestARefusedKeepLeavesThePasteSendable(t *testing.T) {
	m, k := testModel(t)
	k.refuseText = true
	m.Update(tea.PasteMsg{Content: longPaste("GAMMA")})
	run(m, press(m, "ctrl+g"))
	if got := m.composer.Value(); !strings.Contains(got, "[Pasted text #1") {
		t.Fatalf("composer = %q", got)
	}
	run(m, press(m, "enter"))
	if !strings.Contains(strings.Join(k.seen(), "\n"), "GAMMA-line-3") {
		t.Fatalf("the paste was not sent whole after a refusal: %q", k.seen())
	}
}

// A resumed conversation numbers past kept pastes too.
func TestKeptPasteLabelsCountForNumbering(t *testing.T) {
	var p pasteStore
	p.seed([]HistoryMessage{{Content: "[Pasted file #4 · 9 lines] go"}})
	if p.next != 4 {
		t.Fatalf("next = %d", p.next)
	}
}
