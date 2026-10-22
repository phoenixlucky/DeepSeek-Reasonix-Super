package websearch

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const searxngProbeTimeout = 8 * time.Second

// SearxngProbe is what one test query to an instance showed. Code is empty when
// the instance answered; Status is the HTTP status when that is what refused.
type SearxngProbe struct {
	OK     bool   `json:"ok"`
	Code   string `json:"code,omitempty"`
	Status int    `json:"status,omitempty"`
}

// ProbeSearxng sends one search to the address the user saved and reports why it
// did not answer. It follows no redirect, so that address is the only host it
// contacts; it uses no proxy, so the address it checks is the one it connects
// to; it connects only to an address that is not link-local, unspecified or a
// metadata endpoint; and it is bounded in time and in body size.
func ProbeSearxng(ctx context.Context, saved string) (SearxngProbe, error) {
	return probeSearxng(ctx, saved, systemLookup)
}

func probeSearxng(ctx context.Context, saved string, lookup lookupFunc) (SearxngProbe, error) {
	client := &http.Client{
		Timeout:       searxngProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{DialContext: guardedDial(lookup)},
	}
	backend, err := NewSearxng(saved, client)
	if err != nil {
		return SearxngProbe{}, err
	}
	_, err = backend.Search(ctx, "searxng")
	var failed *BackendError
	switch {
	case err == nil, errors.As(err, &failed) && failed.Kind == FailEmpty:
		return SearxngProbe{OK: true}, nil
	case errors.Is(err, errAddressRefused):
		return SearxngProbe{Code: CodeSearxngAddressRefused}, nil
	case errors.As(err, &failed):
		return SearxngProbe{Code: failed.Code, Status: failed.Status}, nil
	}
	return SearxngProbe{Code: CodeSearxngUnreachable}, nil
}
