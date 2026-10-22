package tui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/contract/eventwire"
)

// inboxKernel is a kernel whose inbox holds what the test puts in it and which
// records the changes the keys ask for.
type inboxKernel struct {
	mu     sync.Mutex
	items  []InboxItem
	bodies map[string]string
	paused bool
	calls  []string
	// failBody makes a read of a message's full text fail.
	failBody bool
}

func (k *inboxKernel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	k.mu.Lock()
	defer k.mu.Unlock()
	raw, _ := io.ReadAll(r.Body)
	k.calls = append(k.calls, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(raw)))
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/inbox":
		_ = json.NewEncoder(w).Encode(InboxSnapshot{Revision: 1, Paused: k.paused, Items: k.items})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/inbox/items/") && k.failBody:
		w.WriteHeader(http.StatusInternalServerError)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/inbox/items/"):
		id := strings.TrimPrefix(r.URL.Path, "/inbox/items/")
		_ = json.NewEncoder(w).Encode(map[string]any{"envelope": map[string]string{"submitText": k.bodies[id]}})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (k *inboxKernel) seen() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.calls...)
}

func (k *inboxKernel) did(prefix string) bool {
	for _, c := range k.seen() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func inboxModel(t *testing.T, texts ...string) (*model, *inboxKernel) {
	t.Helper()
	k := &inboxKernel{bodies: map[string]string{}}
	for i, text := range texts {
		id := "q-" + string(rune('a'+i))
		k.items = append(k.items, InboxItem{ID: id, Intent: "followup", State: "queued", Preview: text})
		k.bodies[id] = text + " (full)"
	}
	srv := httptest.NewServer(k)
	t.Cleanup(srv.Close)
	m := newModel(context.Background(), Options{Client: &Client{HTTP: srv.Client(), Base: srv.URL}})
	closed := make(chan Update)
	close(closed)
	m.updates = closed
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	startTurn(m)
	run(m, m.fetchQueue())
	return m, k
}

func keyUp(m *model) tea.Cmd   { _, c := m.Update(tea.KeyPressMsg{Code: tea.KeyUp}); return c }
func keyDown(m *model) tea.Cmd { _, c := m.Update(tea.KeyPressMsg{Code: tea.KeyDown}); return c }
func altUp(m *model) tea.Cmd {
	_, c := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt})
	return c
}
func letter(m *model, r rune) tea.Cmd {
	_, c := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return c
}

func rowsText(m *model) string { return strings.Join(m.queueRows(), "\n") }

func TestQueuedMessagesAreDrawnAsNumberedRowsAndCounted(t *testing.T) {
	m, _ := inboxModel(t, "first", "second", "third", "fourth", "fifth")
	rows := rowsText(m)
	for _, want := range []string{"[1] first", "[2] second", "[3] third", "+2 more"} {
		if !strings.Contains(rows, want) {
			t.Fatalf("rows lack %q:\n%s", want, rows)
		}
	}
	if strings.Contains(rows, "fourth") {
		t.Fatalf("only %d rows are drawn:\n%s", queueRowLimit, rows)
	}
	if !strings.Contains(m.workingLine(), "5 in inbox") {
		t.Fatalf("the working line says nothing of the queue: %q", m.workingLine())
	}
}

func TestAPendingRowTheQueueAlreadyDrawsIsNotDrawnTwice(t *testing.T) {
	m, _ := inboxModel(t, "alpha")
	row := m.tr.AddUser("alpha")
	m.tr.Items[len(m.tr.Items)-1].Pending = true
	m.tr.SetQueueID(row, "q-a")
	if got := strings.Join(m.liveLines(), "\n"); strings.Contains(got, "⧗") {
		t.Fatalf("the queued message is drawn as a pending row and as a queue row:\n%s", got)
	}
}

func TestUpSelectsTheNewestQueuedMessageIntoTheComposerAndDownPutsTheDraftBack(t *testing.T) {
	m, _ := inboxModel(t, "one", "two")
	typeText(m, "my draft")
	run(m, keyUp(m))
	if got := m.composer.Value(); got != "two (full)" || !strings.Contains(rowsText(m), "▸ [2] two") {
		t.Fatalf("composer %q rows:\n%s", got, rowsText(m))
	}
	run(m, keyUp(m))
	if got := m.composer.Value(); got != "one (full)" {
		t.Fatalf("second up selected %q", got)
	}
	run(m, keyDown(m))
	run(m, keyDown(m))
	if got := m.composer.Value(); got != "my draft" || m.queue.selID != "" {
		t.Fatalf("past the newest the draft should be back, composer %q selected %q", got, m.queue.selID)
	}
}

func TestUpIsStillHistoryWhenNothingIsQueuedOrNoTurnRuns(t *testing.T) {
	m, _ := inboxModel(t, "one")
	apply(m, eventwire.Event{Kind: "turn_done"})
	typeText(m, "sent before")
	run(m, press(m, "enter"))
	run(m, keyUp(m))
	if m.queue.selID != "" {
		t.Fatal("up selected a queued message although no turn runs and the queue is not paused")
	}
}

func TestAltUpMovesTheSelectedMessageAndDeleteNeedsTwoPresses(t *testing.T) {
	m, k := inboxModel(t, "one", "two")
	run(m, keyUp(m))
	run(m, altUp(m))
	if !k.did(`POST /inbox/move {"id":"q-b","toIndex":0}`) {
		t.Fatalf("no move was asked for: %v", k.seen())
	}
	run(m, letter(m, 'd'))
	if k.did("DELETE ") {
		t.Fatal("one d deleted the message")
	}
	run(m, letter(m, 'd'))
	if !k.did("DELETE /inbox/items/q-b") {
		t.Fatalf("the second d did not delete: %v", k.seen())
	}
	if m.queue.selID != "" {
		t.Fatal("the selection outlived the message")
	}
}

func TestDKeyIsTextOnceTheSelectedMessageHasBeenEdited(t *testing.T) {
	m, k := inboxModel(t, "one")
	run(m, keyUp(m))
	typeText(m, "x")
	run(m, letter(m, 'd'))
	if k.did("DELETE ") {
		t.Fatal("d deleted a message the person was editing")
	}
	if got := m.composer.Value(); got != "one (full)xd" {
		t.Fatalf("composer %q: d should have been typed", got)
	}
}

func TestEnterSavesTheEditedMessageInPlaceAndRestoresTheDraft(t *testing.T) {
	m, k := inboxModel(t, "one")
	typeText(m, "draft")
	run(m, keyUp(m))
	m.composer.SetValue("one edited")
	run(m, press(m, "enter"))
	if !k.did(`PATCH /inbox/items/q-a {"input":"one edited"}`) {
		t.Fatalf("the edit was not saved: %v", k.seen())
	}
	if k.did("POST /inbox/items ") {
		t.Fatal("the edit was queued as a new message")
	}
	if m.composer.Value() != "draft" || m.queue.selID != "" {
		t.Fatalf("composer %q selected %q", m.composer.Value(), m.queue.selID)
	}
}

func TestEscLetsGoOfTheSelectionWithoutChangingTheQueue(t *testing.T) {
	m, k := inboxModel(t, "one")
	typeText(m, "keep")
	run(m, keyUp(m))
	run(m, press(m, "esc"))
	if m.composer.Value() != "keep" || m.queue.selID != "" || k.did("PATCH ") || k.did("DELETE ") {
		t.Fatalf("composer %q selected %q calls %v", m.composer.Value(), m.queue.selID, k.seen())
	}
}

func TestSpacePausesTheQueueOnlyWhileTheComposerIsEmpty(t *testing.T) {
	m, k := inboxModel(t, "one")
	run(m, func() tea.Msg { _, c := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}); return c() })
	if !k.did("POST /inbox/pause") {
		t.Fatalf("space did not pause: %v", k.seen())
	}
	typeText(m, "a")
	before := len(k.seen())
	m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if len(k.seen()) != before {
		t.Fatal("space paused the queue while a message was being typed")
	}
}

func TestDeletingAQueuedMessageDropsItsPendingRow(t *testing.T) {
	m, _ := inboxModel(t, "one")
	row := m.tr.AddUser("one")
	m.tr.Items[len(m.tr.Items)-1].Pending = true
	m.tr.SetQueueID(row, "q-a")
	m.tr.DropQueued("q-a")
	for _, it := range m.tr.Items {
		if it.Pending && it.QueueID == "q-a" {
			t.Fatal("the deleted message is still a pending row")
		}
	}
}

// The selection shows the full message only once it has been read: until then the
// composer holds nothing of it, so nothing can be saved over the message.
func TestEnterBeforeTheFullTextArrivesSavesNothingAndSendsNothing(t *testing.T) {
	m, k := inboxModel(t, "one")
	typeText(m, "draft")
	keyUp(m) // the read of the body is started but its answer not yet fed back
	if got := m.composer.Value(); got == "one" || strings.Contains(got, "one") {
		t.Fatalf("the truncated preview was put in the composer: %q", got)
	}
	run(m, press(m, "enter"))
	if k.did("PATCH ") || k.did("POST /inbox/items ") || k.did("POST /submit") {
		t.Fatalf("enter before the text arrived changed or sent something: %v", k.seen())
	}
}

func TestAFailedReadOfTheFullTextLetsGoOfTheSelectionAndSaysSo(t *testing.T) {
	m, k := inboxModel(t, "one")
	k.failBody = true
	typeText(m, "draft")
	run(m, keyUp(m))
	if m.queue.selID != "" || m.composer.Value() != "draft" {
		t.Fatalf("selected %q composer %q: a message that could not be read must not stay selected", m.queue.selID, m.composer.Value())
	}
	run(m, press(m, "enter"))
	if k.did("PATCH ") {
		t.Fatalf("a message whose text was never read was saved over: %v", k.seen())
	}
}

func TestAMessageThatLeavesTheQueueWhileSelectedEndsTheEditAndBringsTheDraftBack(t *testing.T) {
	m, k := inboxModel(t, "one", "two")
	typeText(m, "my draft")
	run(m, keyUp(m))
	if m.queue.selID == "" {
		t.Fatal("nothing was selected")
	}
	k.mu.Lock()
	k.items = k.items[:1]
	k.mu.Unlock()
	run(m, m.fetchQueue())
	if m.queue.selID != "" || m.composer.Value() != "my draft" {
		t.Fatalf("selected %q composer %q: the message already ran, so the edit ends and the draft returns", m.queue.selID, m.composer.Value())
	}
	run(m, press(m, "enter"))
	if k.did("PATCH ") {
		t.Fatalf("enter saved an edit to a message that already ran: %v", k.seen())
	}
}
