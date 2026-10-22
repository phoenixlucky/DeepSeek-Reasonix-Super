package tui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBtwAsksBesideTheSessionAndSaysNothingWasSaved(t *testing.T) {
	m, k := queueModel(t, map[string]any{
		"POST /side":        map[string]string{"id": "s1"},
		"POST /side/s1/ask": map[string]string{"answer": "It is a plain function."},
	})
	m.tr.Running = true
	queueSend(m, "/btw what does this function do?")
	got := queueLastNotice(m)
	if !strings.Contains(got, "It is a plain function.") || !strings.Contains(got, "not saved") {
		t.Fatalf("notice = %q, want the answer marked as not saved", got)
	}
	calls := k.seen()
	for _, want := range []string{`POST /side {}`, `POST /side/s1/ask {"question":"what does this function do?"`, `DELETE /side/s1`} {
		if !strings.Contains(calls, want) {
			t.Fatalf("missing %q in:\n%s", want, calls)
		}
	}
	if strings.Contains(calls, "/submit") || strings.Contains(calls, "/inbox") {
		t.Fatalf("/btw reached the main conversation:\n%s", calls)
	}
	for _, it := range m.tr.Items {
		if it.Pending {
			t.Fatalf("/btw left a queued row: %+v", it)
		}
	}
}

func TestBtwWithoutAQuestionShowsUsageAndAsksNothing(t *testing.T) {
	m, k := queueModel(t, nil)
	queueSend(m, "/btw")
	if got := queueLastNotice(m); !strings.HasPrefix(got, "usage: /btw") || k.seen() != "" {
		t.Fatalf("notice = %q, calls:\n%s", got, k.seen())
	}
}

type sideRefuser struct{ code string }

func (s sideRefuser) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/side" {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"` + s.code + `","error":"x"}`))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func TestBtwRefusalsAreSaidFromTheirIdentity(t *testing.T) {
	for code, says := range map[string]string{
		"side.limit":       "too many side questions",
		"side.unavailable": "not available",
	} {
		srv := httptest.NewServer(sideRefuser{code: code})
		c := &Client{HTTP: srv.Client(), Base: srv.URL}
		_, err := c.Side(t.Context(), "q")
		srv.Close()
		if Code(err) != code {
			t.Fatalf("%s: err = %v", code, err)
		}
		if got := btwFailure(err); !strings.Contains(got, says) {
			t.Fatalf("%s said %q, want it to mention %q", code, got, says)
		}
	}
}

func TestBtwClosesTheSideConversationWhenTheQuestionFails(t *testing.T) {
	m, k := queueModel(t, map[string]any{"POST /side": map[string]string{"id": "s2"}})
	m.tr.Running = false
	queueSend(m, "/btw anything")
	if !strings.Contains(k.seen(), "DELETE /side/s2") {
		t.Fatalf("the side conversation was left open:\n%s", k.seen())
	}
}

func TestBtwSpeaksTheInterfaceLanguage(t *testing.T) {
	inChinese(t)
	m, _ := queueModel(t, map[string]any{
		"POST /side":        map[string]string{"id": "s1"},
		"POST /side/s1/ask": map[string]string{"answer": "答案"},
	})
	queueSend(m, "/btw 这是什么？")
	if got := queueLastNotice(m); !strings.Contains(got, "未保存到本会话") || !strings.Contains(got, "答案") {
		t.Fatalf("answer notice = %q", got)
	}
	queueSend(m, "/btw")
	if got := queueLastNotice(m); !strings.HasPrefix(got, "用法：/btw") {
		t.Fatalf("usage notice = %q", got)
	}
	for code, says := range map[string]string{"side.limit": "旁路提问太多", "side.unavailable": "不支持旁路提问"} {
		srv := httptest.NewServer(sideRefuser{code: code})
		c := &Client{HTTP: srv.Client(), Base: srv.URL}
		_, err := c.Side(t.Context(), "q")
		srv.Close()
		if got := btwFailure(err); !strings.Contains(got, says) {
			t.Fatalf("%s said %q, want it to mention %q", code, got, says)
		}
	}
}
