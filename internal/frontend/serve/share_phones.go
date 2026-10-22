package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"reasonix/internal/platform/account"
	"reasonix/internal/platform/remotecloud"
)

// CloudPhones is how the host-only sharing panel reaches the devices enrolled
// against this Studio at the account service. The host supplies it once it has
// the account connection; without it the routes answer 409 `phones.unavailable`.
type CloudPhones interface {
	Phones(ctx context.Context, state string) ([]account.Phone, error)
	ResolvePhone(ctx context.Context, id, action, replaceID string) error
	RevokePhone(ctx context.Context, id string) error
}

const (
	codePhonesUnavailable   = "phones.unavailable"
	codePhonesNotRegistered = "phones.not_registered"
	codePhonesNotFound      = "phones.not_found"
	codePhonesRefused       = "phones.refused"
	codePhonesAccount       = "phones.account_unreachable"
)

// PhoneView is one enrolled device as the panel draws it.
type PhoneView struct {
	ID               string `json:"id"`
	Ordinal          int    `json:"ordinal,omitempty"`
	Name             string `json:"name"`
	State            string `json:"state"`
	UAClass          string `json:"uaClass"`
	CreatedAt        string `json:"createdAt"`
	LastSeenAt       string `json:"lastSeenAt"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	KeyThumbprint    string `json:"keyThumbprint"`
	ReplaceCandidate string `json:"replaceCandidate,omitempty"`
}

// PhoneList is what the panel reads: the devices that are waiting for the person
// and the ones that are in, with revoked ones left out.
type PhoneList struct {
	Phones []PhoneView `json:"phones"`
}

func phoneView(p account.Phone) PhoneView {
	v := PhoneView{ID: p.ID, Name: p.Name, State: p.State, UAClass: p.UAClass, CreatedAt: p.CreatedAt, LastSeenAt: p.LastSeenAt, KeyThumbprint: p.KeyThumbprint}
	if p.Ordinal != nil {
		v.Ordinal = *p.Ordinal
	}
	if p.ExpiresAt != nil {
		v.ExpiresAt = *p.ExpiresAt
	}
	if p.ReplaceCandidate != nil {
		v.ReplaceCandidate = *p.ReplaceCandidate
	}
	return v
}

// SetCloudPhones attaches the account connection the phones panel reads and writes through.
func (h *Hub) SetCloudPhones(p CloudPhones) {
	if h.opts.Share != nil {
		h.opts.Share.setCloudPhones(p)
	}
}

func (s *DeviceShare) setCloudPhones(p CloudPhones) {
	s.mu.Lock()
	s.cloudPhones = p
	s.mu.Unlock()
}

func (s *DeviceShare) phones() CloudPhones {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cloudPhones
}

const phonesTimeout = 20 * time.Second

func (h *Hub) registerPhoneRoutes(mux *http.ServeMux) {
	if h.opts.Share == nil {
		return
	}
	mux.HandleFunc("GET /share/phones", h.sharePhones)
	mux.HandleFunc("POST /share/phones/resolve", h.sharePhoneResolve)
	mux.HandleFunc("POST /share/phones/revoke", h.sharePhoneRevoke)
}

// refusePhones says why the account service did not do it by what it answered,
// never by reading its words: not registered, not found, refused (a typed
// answer from the service), or not reachable at all.
func refusePhones(w http.ResponseWriter, err error) {
	var apiErr *account.Error
	switch {
	case errors.Is(err, remotecloud.ErrNotRegistered):
		refuse(w, http.StatusConflict, codePhonesNotRegistered, "this Studio is not registered with an account", nil)
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound:
		refuse(w, http.StatusNotFound, codePhonesNotFound, "no such phone", map[string]any{"code": apiErr.Code})
	case errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500:
		refuse(w, http.StatusConflict, codePhonesRefused, "the account service refused it", map[string]any{"code": apiErr.Code})
	default:
		refuse(w, http.StatusBadGateway, codePhonesAccount, "the account service did not answer", nil)
	}
}

func (h *Hub) sharePhones(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	src := h.opts.Share.phones()
	if src == nil {
		refuse(w, http.StatusConflict, codePhonesUnavailable, "phones are not available on this kernel", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), phonesTimeout)
	defer cancel()
	all, err := src.Phones(ctx, "all")
	if err != nil {
		refusePhones(w, err)
		return
	}
	out := PhoneList{Phones: []PhoneView{}}
	for _, p := range all {
		if p.State != "revoked" {
			out.Phones = append(out.Phones, phoneView(p))
		}
	}
	writeJSON(w, out)
}

func (h *Hub) sharePhoneResolve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string `json:"id"`
		Action    string `json:"action"`
		ReplaceID string `json:"replaceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		missingField(w, "id")
		return
	}
	switch body.Action {
	case account.PhoneAdd, account.PhoneReplace, account.PhoneReject:
	default:
		missingField(w, "action")
		return
	}
	src := h.opts.Share.phones()
	if src == nil {
		refuse(w, http.StatusConflict, codePhonesUnavailable, "phones are not available on this kernel", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), phonesTimeout)
	defer cancel()
	if err := src.ResolvePhone(ctx, body.ID, body.Action, body.ReplaceID); err != nil {
		refusePhones(w, err)
		return
	}
	h.sharePhones(w, r)
}

func (h *Hub) sharePhoneRevoke(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		missingField(w, "id")
		return
	}
	src := h.opts.Share.phones()
	if src == nil {
		refuse(w, http.StatusConflict, codePhonesUnavailable, "phones are not available on this kernel", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), phonesTimeout)
	defer cancel()
	if err := src.RevokePhone(ctx, body.ID); err != nil {
		refusePhones(w, err)
		return
	}
	h.sharePhones(w, r)
}
