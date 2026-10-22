package remotecloud

import (
	"context"
	"errors"

	"reasonix/internal/platform/account"
)

// ErrNotRegistered: this Studio has not registered with the account, so it has
// no device credential to speak for itself with. Signing in registers it.
var ErrNotRegistered = errors.New("remote cloud: this Studio is not registered with an account")

func (h *Host) deviceAuth() (account.DeviceAuth, error) {
	saved, err := h.saved()
	if err != nil {
		return account.DeviceAuth{}, err
	}
	if saved == nil {
		return account.DeviceAuth{}, ErrNotRegistered
	}
	return account.DeviceAuth{DeviceID: saved.DeviceID, Credential: saved.DeviceCredential}, nil
}

// NotifyPhones tells this desktop's enrolled phones that something is waiting.
// kind is one of account.PushApproval, PushInput and PushError: a type and a
// count are all that leave this machine.
func (h *Host) NotifyPhones(ctx context.Context, kind string, count int) error {
	auth, err := h.deviceAuth()
	if err != nil {
		return err
	}
	_, err = h.client.NotifyPhones(ctx, auth, kind, count)
	return err
}

// Phones lists the devices enrolled against this desktop in a state (pending,
// active, revoked or all).
func (h *Host) Phones(ctx context.Context, state string) ([]account.Phone, error) {
	auth, err := h.deviceAuth()
	if err != nil {
		return nil, err
	}
	return h.client.Phones(ctx, auth, state)
}

// ResolvePhone answers a device waiting for approval.
func (h *Host) ResolvePhone(ctx context.Context, id, action, replaceID string) error {
	auth, err := h.deviceAuth()
	if err != nil {
		return err
	}
	return h.client.ResolvePhone(ctx, auth, id, action, replaceID)
}

// RevokePhone ends an enrolled device.
func (h *Host) RevokePhone(ctx context.Context, id string) error {
	auth, err := h.deviceAuth()
	if err != nil {
		return err
	}
	return h.client.RevokePhone(ctx, auth, id)
}
