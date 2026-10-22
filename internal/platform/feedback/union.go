package feedback

import (
	"context"
	"net/http"
)

// accountView is what the account lists across its linked installs: the
// reports, each saying whether this install filed it, and the installs.
type accountView struct {
	Installs []LinkedInstall `json:"installs"`
	Items    []struct {
		Item
		LinkedHere bool `json:"linkedHere"`
	} `json:"items"`
}

// siblingItems reads the reports of the other installs of the account this
// install linked to. It is best-effort: signed out, refused or unreachable, the
// person still has this install's own list. What the service says about whether
// this install is linked is remembered, so a link removed elsewhere stops the read.
func (s *Service) siblingItems(ctx context.Context, st state) []Item {
	if !st.Linked || s.assert == nil {
		return nil
	}
	assertion, err := s.assert(ctx)
	if err != nil {
		return nil
	}
	var view accountView
	if err := s.linkCall(ctx, http.MethodGet, "/v1/feedback/account/mine", assertion, &st, &view); err != nil {
		return nil
	}
	here := false
	for _, in := range view.Installs {
		here = here || in.Current
	}
	if !here {
		_ = s.store.update(func(st *state) error { st.Linked = false; return nil })
		return nil
	}
	var out []Item
	for _, it := range view.Items {
		if it.LinkedHere {
			continue
		}
		it.OtherInstall = true
		out = append(out, it.Item)
	}
	return out
}
