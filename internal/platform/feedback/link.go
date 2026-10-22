package feedback

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

const assertionHeader = "X-Account-Assertion"

// LinkedInstall is one install an account holds. ID is the short form the
// service shows; Current marks the install asking.
type LinkedInstall struct {
	ID       string    `json:"id"`
	LinkedAt time.Time `json:"linkedAt"`
	Current  bool      `json:"current"`
}

// Link ties this install to the account the assertion proves. It never runs on
// its own: the person asks for it, and the assertion is spent on that one call.
func (s *Service) Link(ctx context.Context) error {
	st, err := s.linkable()
	if err != nil {
		return err
	}
	assertion, err := s.assertion(ctx)
	if err != nil {
		return err
	}
	if err := s.linkCall(ctx, http.MethodPost, "/v1/feedback/link", assertion, &st, nil); err != nil {
		return err
	}
	return s.store.update(func(st *state) error { st.Linked = true; return nil })
}

// Unlink removes this install's link. Unlinking changes nothing else: the
// install keeps its level and its quota.
func (s *Service) Unlink(ctx context.Context) error {
	st, err := s.linkable()
	if err != nil {
		return err
	}
	assertion, err := s.assertion(ctx)
	if err != nil {
		return err
	}
	if err := s.linkCall(ctx, http.MethodDelete, "/v1/feedback/link", assertion, &st, nil); err != nil {
		return err
	}
	return s.store.update(func(st *state) error { st.Linked = false; return nil })
}

// UnlinkInstall removes one of the account's links by its short id, from any
// install: the account is the authority, so no install proof is sent.
func (s *Service) UnlinkInstall(ctx context.Context, shortID string) error {
	if !shortInstallID(shortID) {
		return &InvalidError{Field: FieldInstall, Reason: ReasonFormat}
	}
	assertion, err := s.assertion(ctx)
	if err != nil {
		return err
	}
	return s.linkCall(ctx, http.MethodDelete, "/v1/feedback/link/"+url.PathEscape(shortID), assertion, nil, nil)
}

// Links lists the installs the account holds. It carries this install's proof
// when there is one, so the answer can say which row is this install.
func (s *Service) Links(ctx context.Context) ([]LinkedInstall, error) {
	assertion, err := s.assertion(ctx)
	if err != nil {
		return nil, err
	}
	st, _ := s.store.load()
	var proof *state
	if st.InstallID != "" && st.InstallToken != "" {
		proof = &st
	}
	var out struct {
		Installs []LinkedInstall `json:"installs"`
	}
	if err := s.linkCall(ctx, http.MethodGet, "/v1/feedback/link", assertion, proof, &out); err != nil {
		return nil, err
	}
	return out.Installs, nil
}

// assertion mints the proof of the signed-in account for one call. It is asked
// for per action and dropped afterwards; a service built without a way to sign
// in has no account to prove.
func (s *Service) assertion(ctx context.Context) (string, error) {
	if s.assert == nil {
		return "", ErrAccountRequired
	}
	return s.assert(ctx)
}

// linkable is the install a link would name; one that never sent a report has
// no token to prove itself with.
func (s *Service) linkable() (state, error) {
	st, err := s.store.load()
	if err != nil {
		return state{}, err
	}
	if st.InstallID == "" || st.InstallToken == "" {
		return state{}, ErrNoInstall
	}
	return st, nil
}

func (s *Service) linkCall(ctx context.Context, method, path, assertion string, install *state, out any) error {
	if out == nil {
		out = &struct{}{}
	}
	err := s.withRetry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, method, s.base+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set(assertionHeader, assertion)
		if install != nil {
			req.Header.Set("X-Install-Id", install.InstallID)
			req.Header.Set("X-Install-Token", install.InstallToken)
		}
		return s.do(req, out)
	}, false)
	if errors.Is(err, ErrBadToken) && install != nil {
		_ = s.store.update(func(st *state) error { st.retire(); return nil })
	}
	return err
}

// shortInstallID is the twelve lowercase hex digits the service shows for an install.
func shortInstallID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
