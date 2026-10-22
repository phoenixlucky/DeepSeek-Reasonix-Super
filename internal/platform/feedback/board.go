package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// BoardEntry is one person on the contribution board: the name they chose, how
// many of their suggestions shipped, where, and (in a release view only) their
// place. Nothing else about them is ever on the wire.
type BoardEntry struct {
	DisplayName string         `json:"displayName"`
	Credits     int            `json:"credits"`
	Rank        *int           `json:"rank"`
	Releases    []BoardRelease `json:"releases"`
}

// BoardRelease is how many of a person's suggestions shipped in one release.
type BoardRelease struct {
	Version string `json:"version"`
	Count   int    `json:"count"`
}

// Board is the public contribution board. Available is false where the service
// serves none, which hides the section rather than failing it.
type Board struct {
	Available bool         `json:"available"`
	View      string       `json:"view"`
	Release   *string      `json:"release"`
	Entries   []BoardEntry `json:"entries"`
}

// BoardMe is what the signed-in person has chosen about being on the board.
type BoardMe struct {
	Listed      bool    `json:"listed"`
	DisplayName *string `json:"displayName"`
	// Delisted: a maintainer removed this account, which cannot list itself
	// again until it is restored.
	Delisted bool `json:"delisted"`
}

// Board reads the public board, all releases or one. It needs no account. A
// service without the route answers Available false, so the section is simply
// not offered.
func (s *Service) Board(ctx context.Context, release string) (Board, error) {
	path := "/v1/feedback/board"
	if release != "" {
		path += "?release=" + url.QueryEscape(release)
	}
	var out Board
	err := s.withRetry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
		if err != nil {
			return err
		}
		return s.do(req, &out)
	}, false)
	if errors.Is(err, ErrRouteAbsent) {
		return Board{Available: false, View: "all", Entries: []BoardEntry{}}, nil
	}
	if err != nil {
		return Board{}, err
	}
	out.Available = true
	if out.Entries == nil {
		out.Entries = []BoardEntry{}
	}
	return out, nil
}

// BoardMe reads the account's own choice. It is the person's own question: the
// assertion is spent on this one call.
func (s *Service) BoardMe(ctx context.Context) (BoardMe, error) {
	assertion, err := s.assertion(ctx)
	if err != nil {
		return BoardMe{}, err
	}
	var out BoardMe
	err = s.linkCall(ctx, http.MethodGet, "/v1/feedback/board/me", assertion, nil, &out)
	return out, err
}

// JoinBoard puts the account on the board under the name given, which only the
// person chooses. The service decides whether the name may be shown.
func (s *Service) JoinBoard(ctx context.Context, displayName string) (BoardMe, error) {
	assertion, err := s.assertion(ctx)
	if err != nil {
		return BoardMe{}, err
	}
	var out BoardMe
	if err := s.boardWrite(ctx, http.MethodPut, assertion, `{"displayName":`+jsonString(displayName)+`}`, &out); err != nil {
		return BoardMe{}, err
	}
	return s.BoardMe(ctx)
}

// LeaveBoard takes the account off the board.
func (s *Service) LeaveBoard(ctx context.Context) (BoardMe, error) {
	assertion, err := s.assertion(ctx)
	if err != nil {
		return BoardMe{}, err
	}
	var out struct{}
	if err := s.boardWrite(ctx, http.MethodDelete, assertion, "", &out); err != nil {
		return BoardMe{}, err
	}
	return s.BoardMe(ctx)
}

func jsonString(v string) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// boardWrite sends the account's own choice to the board's opt-in route. Both
// verbs are idempotent, so an unreachable service is simply tried again.
func (s *Service) boardWrite(ctx context.Context, method, assertion, body string, out any) error {
	return s.withRetry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, method, s.base+"/v1/feedback/board/optin", strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set(assertionHeader, assertion)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		return s.do(req, out)
	}, false)
}
