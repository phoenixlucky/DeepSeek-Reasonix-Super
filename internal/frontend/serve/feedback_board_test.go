package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/platform/feedback"
	"reasonix/internal/session/control"
)

// boardCalls counts what reached the controller: a refused request must reach nothing.
type boardCalls struct {
	control.SessionAPI
	read, me, join, leave int
	joined                string
	err                   error
}

func (b *boardCalls) FeedbackBoard(_ context.Context, _ string) (feedback.Board, error) {
	b.read++
	return feedback.Board{Available: true, View: "all", Entries: []feedback.BoardEntry{}}, nil
}

func (b *boardCalls) FeedbackBoardMe(_ context.Context) (feedback.BoardMe, error) {
	b.me++
	return feedback.BoardMe{}, b.err
}

func (b *boardCalls) JoinFeedbackBoard(_ context.Context, name string) (feedback.BoardMe, error) {
	b.join++
	b.joined = name
	return feedback.BoardMe{Listed: b.err == nil}, b.err
}

func (b *boardCalls) LeaveFeedbackBoard(_ context.Context) (feedback.BoardMe, error) {
	b.leave++
	return feedback.BoardMe{}, nil
}

func boardRoute(b *boardCalls, method, path, body string, device bool) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	(&Server{ctrl: b}).registerFeedbackRoutes(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if device {
		req = req.WithContext(withDeviceReach(req.Context(), "phone-1", 1))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAPairedDeviceCannotJoinLeaveOrReadTheWindowsBoardChoiceButCanReadTheBoard(t *testing.T) {
	routes := []struct {
		name, method, path, body string
		count                    func(*boardCalls) int
	}{
		{"read the account's choice", http.MethodGet, "/feedback/board/me", "", func(b *boardCalls) int { return b.me }},
		{"join", http.MethodPost, "/feedback/board/join", `{"displayName":"Sample"}`, func(b *boardCalls) int { return b.join }},
		{"leave", http.MethodPost, "/feedback/board/leave", "", func(b *boardCalls) int { return b.leave }},
	}
	for _, rt := range routes {
		device := &boardCalls{}
		rec := boardRoute(device, rt.method, rt.path, rt.body, true)
		var reason Reason
		_ = json.Unmarshal(rec.Body.Bytes(), &reason)
		if rec.Code != http.StatusNotFound || reason.Code != codeDeviceHostOnly || rt.count(device) != 0 {
			t.Errorf("%s from a device: status %d code %q reached the controller %d times, want 404 %s and never", rt.name, rec.Code, reason.Code, rt.count(device), codeDeviceHostOnly)
		}
		host := &boardCalls{}
		rec = boardRoute(host, rt.method, rt.path, rt.body, false)
		if rec.Code == http.StatusNotFound || rt.count(host) != 1 {
			t.Errorf("%s from the host: status %d, reached the controller %d times, want it served once", rt.name, rec.Code, rt.count(host))
		}
	}
	device := &boardCalls{}
	if rec := boardRoute(device, http.MethodGet, "/feedback/board?release=v2.1.0", "", true); rec.Code != http.StatusOK || device.read != 1 {
		t.Errorf("a device reading the public board: status %d, reads %d, want it served", rec.Code, device.read)
	}
}

func TestTheBoardRoutesThatActForTheAccountAreInTheOneHostOnlyTable(t *testing.T) {
	for _, r := range []hostRoute{{http.MethodGet, "/feedback/board/me"}, {http.MethodPost, "/feedback/board/join"}, {http.MethodPost, "/feedback/board/leave"}} {
		if !hostOnlyRoute(r.method, r.path) {
			t.Errorf("%s %s is not in hostOnlyRoutes, so the remote-pane proxy would forward it with the host's token", r.method, r.path)
		}
	}
	if hostOnlyRoute(http.MethodGet, "/feedback/board") {
		t.Error("the public board read must stay open to a paired device")
	}
}

func TestEachBoardRefusalAnswersItsOwnCodeAndTheNamePassesThroughUntouched(t *testing.T) {
	for want, err := range map[string]error{
		codeBoardNameInvalid:  feedback.ErrBoardNameInvalid,
		codeBoardNameReserved: feedback.ErrBoardNameReserved,
		codeBoardNameTaken:    feedback.ErrBoardNameTaken,
		codeBoardDelisted:     feedback.ErrBoardDelisted,
		codeBoardNoLink:       feedback.ErrBoardNoLink,
		codeFeedbackAccount:   feedback.ErrAccountRequired,
	} {
		b := &boardCalls{err: err}
		rec := boardRoute(b, http.MethodPost, "/feedback/board/join", `{"displayName":" Sample  User "}`, false)
		var reason Reason
		_ = json.Unmarshal(rec.Body.Bytes(), &reason)
		if reason.Code != want {
			t.Errorf("%v answered %d %q, want %q", err, rec.Code, reason.Code, want)
		}
		if b.joined != " Sample  User " {
			t.Errorf("the name reached the kernel as %q: the service, not the screen, decides what a name may be", b.joined)
		}
	}
	if !errors.Is(feedback.ErrRouteAbsent, feedback.ErrUnavailable) {
		t.Error("an absent route must stay an unavailable service")
	}
}
