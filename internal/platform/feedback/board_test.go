package feedback

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const sampleBoard = `{"view":"release","release":"v2.1.0","entries":[{"displayName":"Sample User","credits":2,"rank":1,"releases":[{"version":"v2.1.0","count":2}]}]}`

func TestTheBoardIsReadWithNoAccountAndAskedForTheReleaseNamed(t *testing.T) {
	stub := &linkStub{body: sampleBoard}
	s := linkService(t, stub, false)
	got, err := s.Board(context.Background(), "v2.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Release == nil || *got.Release != "v2.1.0" || len(got.Entries) != 1 || got.Entries[0].DisplayName != "Sample User" || *got.Entries[0].Rank != 1 {
		t.Fatalf("board = %+v", got)
	}
	req := stub.seen[0]
	if req.Method != http.MethodGet || req.URL.Path != "/v1/feedback/board" || req.URL.Query().Get("release") != "v2.1.0" {
		t.Fatalf("request = %s %s", req.Method, req.URL)
	}
	if req.Header.Get(assertionHeader) != "" || req.Header.Get("X-Install-Token") != "" {
		t.Fatalf("a public read carried credentials: %v", req.Header)
	}
}

func TestAServiceWithNoBoardRouteIsNotOfferedOneAndIsNotAnError(t *testing.T) {
	s := linkService(t, &linkStub{code: http.StatusNotFound, body: "not found"}, false)
	got, err := s.Board(context.Background(), "")
	if err != nil || got.Available || len(got.Entries) != 0 {
		t.Fatalf("board = %+v err = %v, want an empty answer that says unavailable", got, err)
	}
	down := linkService(t, &linkStub{code: http.StatusBadGateway, body: "bad gateway"}, false)
	if _, err := down.Board(context.Background(), ""); err == nil {
		t.Fatal("a failing service must not read as a service with no board")
	}
}

func TestAnAbsentRouteIsStillAnUnavailableServiceToTheOtherCallers(t *testing.T) {
	if !errors.Is(ErrRouteAbsent, ErrUnavailable) {
		t.Fatal("ErrRouteAbsent must keep matching ErrUnavailable so existing callers read it as before")
	}
	s := linkService(t, &linkStub{code: http.StatusNotFound, body: "not found"}, true, "v1.a.b")
	if _, err := s.Links(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("links against a service with no link route: %v", err)
	}
}

func TestJoiningSendsTheNameWithTheAccountAssertionAndReadsBackTheChoice(t *testing.T) {
	var methods []string
	var bodies []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		methods = append(methods, r.Method+" "+r.URL.Path+" "+r.Header.Get(assertionHeader))
		bodies = append(bodies, string(raw))
		_, _ = io.WriteString(w, `{"listed":true,"displayName":"Sample User","delisted":false}`)
	})
	s := linkService(t, h, false, "v1.a.b")
	me, err := s.JoinBoard(context.Background(), `Sample "User"`)
	if err != nil || !me.Listed || me.DisplayName == nil {
		t.Fatalf("me = %+v err = %v", me, err)
	}
	if len(methods) != 2 || methods[0] != "PUT /v1/feedback/board/optin v1.a.b" || methods[1] != "GET /v1/feedback/board/me v1.a.b" {
		t.Fatalf("calls = %v", methods)
	}
	if !strings.Contains(bodies[0], `"displayName":"Sample \"User\""`) {
		t.Fatalf("the name was not sent as JSON: %s", bodies[0])
	}
	if _, err := s.LeaveBoard(context.Background()); err != nil {
		t.Fatal(err)
	}
	if methods[2] != "DELETE /v1/feedback/board/optin v1.a.b" {
		t.Fatalf("leave = %v", methods)
	}
}

func TestEachBoardRefusalKeepsItsOwnIdentity(t *testing.T) {
	for code, want := range map[string]error{
		"feedback.board_name_invalid":  ErrBoardNameInvalid,
		"feedback.board_name_reserved": ErrBoardNameReserved,
		"feedback.board_name_taken":    ErrBoardNameTaken,
		"feedback.board_delisted":      ErrBoardDelisted,
		"feedback.board_no_link":       ErrBoardNoLink,
	} {
		stub := &linkStub{code: http.StatusBadRequest, body: `{"error":{"code":"` + code + `","message":"x"}}`}
		s := linkService(t, stub, false, "v1.a.b")
		if _, err := s.JoinBoard(context.Background(), "Sample"); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", code, err, want)
		}
	}
}

func TestBoardWritesNeedTheAccountBeforeAnythingLeaves(t *testing.T) {
	stub := &linkStub{body: `{}`}
	s := linkService(t, stub, false, "")
	if _, err := s.JoinBoard(context.Background(), "Sample"); !errors.Is(err, ErrAccountRequired) {
		t.Fatalf("join: %v", err)
	}
	if _, err := s.LeaveBoard(context.Background()); !errors.Is(err, ErrAccountRequired) {
		t.Fatalf("leave: %v", err)
	}
	if _, err := s.BoardMe(context.Background()); !errors.Is(err, ErrAccountRequired) {
		t.Fatalf("me: %v", err)
	}
	if len(stub.seen) != 0 {
		t.Fatalf("a request left without an account: %d", len(stub.seen))
	}
}
