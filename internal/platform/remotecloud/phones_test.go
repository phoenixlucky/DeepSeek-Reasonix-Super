package remotecloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/platform/account"
)

func hostWithIdentity(t *testing.T, saved *identity, handler http.HandlerFunc) *Host {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	h := New(account.New(srv.URL, "test", srv.Client()), nil, "", "test")
	h.saved = func() (*identity, error) { return saved, nil }
	return h
}

func TestAStudioThatIsNotRegisteredSpeaksForNobody(t *testing.T) {
	called := false
	h := hostWithIdentity(t, nil, func(http.ResponseWriter, *http.Request) { called = true })
	ctx := context.Background()
	if err := h.NotifyPhones(ctx, account.PushApproval, 1); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("NotifyPhones = %v, want ErrNotRegistered", err)
	}
	if _, err := h.Phones(ctx, "all"); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("Phones = %v", err)
	}
	if err := h.ResolvePhone(ctx, "rc_1", account.PhoneAdd, ""); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("ResolvePhone = %v", err)
	}
	if err := h.RevokePhone(ctx, "rc_1"); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("RevokePhone = %v", err)
	}
	if called {
		t.Fatal("a request was sent without a device credential")
	}
}

func TestARegisteredStudioCallsWithItsOwnDeviceCredential(t *testing.T) {
	var gotID, gotCred, path string
	h := hostWithIdentity(t, &identity{OwnerID: 1, DeviceID: "dev", DeviceCredential: "cred", PrivateKey: "k"}, func(w http.ResponseWriter, r *http.Request) {
		gotID, gotCred, path = r.Header.Get("x-reasonix-device-id"), r.Header.Get("x-reasonix-device-credential"), r.URL.Path
		_, _ = w.Write([]byte(`{"delivered":1,"failed":0,"removed":0}`))
	})
	if err := h.NotifyPhones(context.Background(), account.PushError, 2); err != nil {
		t.Fatal(err)
	}
	if gotID != "dev" || gotCred != "cred" || path != "/host/push/notify" {
		t.Fatalf("sent %q %q %q", gotID, gotCred, path)
	}
}
