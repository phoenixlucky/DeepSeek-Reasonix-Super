package account

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// DeviceAuth is what a registered desktop proves itself with when it calls the
// accounts API on its own behalf: its device id and the credential issued when
// it was registered. It is not an account session.
type DeviceAuth struct {
	DeviceID   string
	Credential string
}

func (a DeviceAuth) headers() (map[string]string, error) {
	if strings.TrimSpace(a.DeviceID) == "" || strings.TrimSpace(a.Credential) == "" {
		return nil, errors.New("account: a device credential is required")
	}
	return map[string]string{"x-reasonix-device-id": a.DeviceID, "x-reasonix-device-credential": a.Credential}, nil
}

// Notification kinds a desktop may report. The accounts API accepts these three
// and nothing else, so no conversation text has anywhere to go.
const (
	PushApproval = "approval"
	PushInput    = "input"
	PushError    = "error"
)

// PushResult says how many phones the notification reached, by outcome; it
// carries no address.
type PushResult struct {
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
	Removed   int `json:"removed"`
}

// NotifyPhones tells the desktop's enrolled phones that something is waiting. kind
// is one of the Push constants and count a number of 1 to 99.
func (c *Client) NotifyPhones(ctx context.Context, auth DeviceAuth, kind string, count int) (*PushResult, error) {
	headers, err := auth.headers()
	if err != nil {
		return nil, err
	}
	var out PushResult
	if err := c.doHeaders(ctx, "POST", "/host/push/notify", headers, map[string]any{"type": kind, "count": count}, &out, 1<<16); err != nil {
		return nil, err
	}
	return &out, nil
}

// Phone is one controller device (a browser) enrolled against this desktop, as
// the desktop sees it. KeyThumbprint is what to show when two look alike.
type Phone struct {
	ID            string  `json:"id"`
	Ordinal       *int    `json:"ordinal"`
	Name          string  `json:"name"`
	NameSource    string  `json:"nameSource"`
	State         string  `json:"state"`
	UAClass       string  `json:"uaClass"`
	ClaimsID      *string `json:"claimsId"`
	CreatedAt     string  `json:"createdAt"`
	LastSeenAt    string  `json:"lastSeenAt"`
	RevokedAt     *string `json:"revokedAt"`
	ExpiresAt     *string `json:"expiresAt"`
	KeyThumbprint string  `json:"keyThumbprint"`
	// ReplaceCandidate is the active device a pending one says it replaces.
	ReplaceCandidate *string `json:"replaceCandidate"`
}

// Phones lists the devices enrolled against this desktop in the given state
// (pending, active, revoked or all).
func (c *Client) Phones(ctx context.Context, auth DeviceAuth, state string) ([]Phone, error) {
	headers, err := auth.headers()
	if err != nil {
		return nil, err
	}
	var out struct {
		Controllers []Phone `json:"controllers"`
	}
	if err := c.doHeaders(ctx, "GET", "/host/controllers?state="+url.QueryEscape(state), headers, nil, &out, 1<<20); err != nil {
		return nil, err
	}
	return out.Controllers, nil
}

// Phone resolution actions: add a waiting device, replace an active one with
// it, or reject it.
const (
	PhoneAdd     = "add"
	PhoneReplace = "replace"
	PhoneReject  = "reject"
)

// ResolvePhone answers a device that is waiting for approval. replaceID names
// the active device that is replaced, and only for PhoneReplace.
func (c *Client) ResolvePhone(ctx context.Context, auth DeviceAuth, id, action, replaceID string) error {
	headers, err := auth.headers()
	if err != nil {
		return err
	}
	if id == "" || strings.ContainsAny(id, "/?#") {
		return fmt.Errorf("account: %q is not a device id", id)
	}
	body := map[string]any{"action": action}
	if action == PhoneReplace {
		body["replaceId"] = replaceID
	}
	return c.doHeaders(ctx, "POST", "/host/controllers/"+id+"/resolve", headers, body, nil, 1<<16)
}

// RevokePhone ends an enrolled device. It cannot be undone: the device starts
// again with a new key.
func (c *Client) RevokePhone(ctx context.Context, auth DeviceAuth, id string) error {
	headers, err := auth.headers()
	if err != nil {
		return err
	}
	if id == "" || strings.ContainsAny(id, "/?#") {
		return fmt.Errorf("account: %q is not a device id", id)
	}
	return c.doHeaders(ctx, "POST", "/host/controllers/"+id+"/revoke", headers, map[string]any{}, nil, 1<<16)
}
