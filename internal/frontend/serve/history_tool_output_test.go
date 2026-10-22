package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/session/control"
)

type historyCtl struct {
	control.SessionAPI
	msgs []provider.Message
}

func (h historyCtl) History() []provider.Message { return h.msgs }

func (historyCtl) StoredAttachments(string) []event.Attachment { return nil }

func (historyCtl) EmbeddedDiffDetectionEnabled() bool { return false }

func historyRoutes(msgs []provider.Message) http.Handler {
	mux := http.NewServeMux()
	(&Server{ctrl: historyCtl{msgs: msgs}}).registerHistoryRoutes(mux)
	return mux
}

func getHistory(h http.Handler, path string, device bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if device {
		req = req.WithContext(withDeviceReach(req.Context(), "phone-1", 1))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// A result with multi-byte text, so a page boundary can land inside a character.
func toolMessages(content string) []provider.Message {
	return []provider.Message{
		{Role: provider.RoleUser, Content: "go"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "bash"}}},
		{Role: provider.RoleTool, ToolCallID: "call-1", Name: "bash", Content: content},
	}
}

func TestToolOutputPagesJoinToWhatHistoryCarriesForThatCall(t *testing.T) {
	content := strings.Repeat("输出行 héllo 😀 end\n", 9000)
	h := historyRoutes(toolMessages(content))

	var fromHistory string
	var all []historyMessage
	if err := json.Unmarshal(getHistory(h, "/history", false).Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.ToolCallID == "call-1" {
			fromHistory = m.Content
		}
	}
	if fromHistory != content {
		t.Fatal("the fixture's history does not carry the content whole")
	}

	var joined strings.Builder
	next, pages := 0, 0
	for {
		rec := getHistory(h, fmt.Sprintf("/history/tool/call-1?offset=%d&limit=1000", next), false)
		if rec.Code != http.StatusOK {
			t.Fatalf("page at %d: %d %s", next, rec.Code, rec.Body.String())
		}
		var page ToolOutputPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Offset != next || page.Bytes != len(fromHistory) || page.CallID != "call-1" {
			t.Fatalf("page = {offset %d bytes %d call %q}, want {%d %d call-1}", page.Offset, page.Bytes, page.CallID, next, len(fromHistory))
		}
		joined.WriteString(page.Text)
		next += len(page.Text)
		pages++
		if !page.Truncated {
			break
		}
		if pages > 10000 {
			t.Fatal("the pages never end")
		}
	}
	if joined.String() != fromHistory {
		t.Fatalf("pages joined to %d bytes that differ from /history's %d", joined.Len(), len(fromHistory))
	}
	if rec := getHistory(h, "/history/tool/call-1", false); rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestToolOutputIsCutOnACharacterAndAWholeCharacterFits(t *testing.T) {
	h := historyRoutes(toolMessages("😀😀😀😀"))
	rec := getHistory(h, "/history/tool/call-1?offset=1&limit=6", false)
	var page ToolOutputPage
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Offset != 0 || page.Text != "😀" || !page.Truncated {
		t.Fatalf("a mid-character offset starts at the character and a page ends on one: %+v", page)
	}
}

func TestToolOutputRefusesWhatItCannotAnswerWithItsOwnCodes(t *testing.T) {
	h := historyRoutes(toolMessages("short"))
	for _, tc := range []struct {
		path, code string
		status     int
	}{
		{"/history/tool/missing", "tool.output_unknown", http.StatusNotFound},
		{"/history/tool/..%2F..%2Fetc%2Fpasswd", "tool.output_unknown", http.StatusNotFound},
		{"/history/tool/call-1?offset=-1", "tool.output_range", http.StatusBadRequest},
		{"/history/tool/call-1?offset=abc", "tool.output_range", http.StatusBadRequest},
		{"/history/tool/call-1?limit=0", "tool.output_range", http.StatusBadRequest},
		{"/history/tool/call-1?limit=x", "tool.output_range", http.StatusBadRequest},
		{"/history/tool/call-1?offset=6", "tool.output_range", http.StatusBadRequest},
	} {
		rec := getHistory(h, tc.path, false)
		var reason Reason
		_ = json.Unmarshal(rec.Body.Bytes(), &reason)
		if rec.Code != tc.status || reason.Code != tc.code {
			t.Errorf("%s: %d %q, want %d %s (%s)", tc.path, rec.Code, reason.Code, tc.status, tc.code, rec.Body.String())
		}
	}
}

func TestToolOutputClampsALargeLimit(t *testing.T) {
	h := historyRoutes(toolMessages(strings.Repeat("a", 400*1024)))
	rec := getHistory(h, "/history/tool/call-1?limit=999999999", false)
	var page ToolOutputPage
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Text) != toolOutputMax || !page.Truncated {
		t.Fatalf("page of %d bytes, truncated=%v, want %d and more to come", len(page.Text), page.Truncated, toolOutputMax)
	}
	if rec := getHistory(h, "/history/tool/call-1", false); func() int { var p ToolOutputPage; _ = json.Unmarshal(rec.Body.Bytes(), &p); return len(p.Text) }() != toolOutputDefault {
		t.Fatal("the default page is not the default size")
	}
}

// The new route holds nothing /history does not: a paired device reaches both or neither.
func TestToolOutputIsReachedExactlyAsHistoryIs(t *testing.T) {
	h := historyRoutes(toolMessages("output"))
	for _, device := range []bool{false, true} {
		hist := getHistory(h, "/history", device)
		tool := getHistory(h, "/history/tool/call-1", device)
		if (hist.Code == http.StatusOK) != (tool.Code == http.StatusOK) {
			t.Errorf("device=%v: /history answered %d and /history/tool %d", device, hist.Code, tool.Code)
		}
	}
	if hostOnlyRoute(http.MethodGet, "/history/tool/call-1") != hostOnlyRoute(http.MethodGet, "/history") {
		t.Fatal("the route's host-only status differs from /history's")
	}
}
