package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type seen struct {
	method, path, query string
	header              http.Header
	body                map[string]any
}

func fakeAccounts(t *testing.T, status int, answer string, record *seen) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*record = seen{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone()}
		if r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&record.body)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "test", srv.Client())
}

var auth = DeviceAuth{DeviceID: strings.Repeat("a", 64), Credential: strings.Repeat("b", 64)}

func TestNotifyPhonesSendsOnlyATypeAndACountWithTheDeviceCredential(t *testing.T) {
	var got seen
	c := fakeAccounts(t, 200, `{"delivered":2,"failed":1,"removed":0}`, &got)
	res, err := c.NotifyPhones(context.Background(), auth, PushApproval, 3)
	if err != nil || res.Delivered != 2 || res.Failed != 1 {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got.method != "POST" || got.path != "/host/push/notify" {
		t.Fatalf("request = %s %s", got.method, got.path)
	}
	if got.header.Get("x-reasonix-device-id") != auth.DeviceID || got.header.Get("x-reasonix-device-credential") != auth.Credential || got.header.Get("Authorization") != "" {
		t.Fatalf("headers = %v: the desktop proves itself with its device credential and no account token", got.header)
	}
	if len(got.body) != 2 || got.body["type"] != "approval" || got.body["count"] != float64(3) {
		t.Fatalf("body = %v, want exactly a type and a count", got.body)
	}
}

func TestADeviceCallWithoutACredentialIsNeverSent(t *testing.T) {
	var got seen
	c := fakeAccounts(t, 200, `{}`, &got)
	for name, call := range map[string]func() error{
		"notify": func() error { _, err := c.NotifyPhones(context.Background(), DeviceAuth{}, PushError, 1); return err },
		"list":   func() error { _, err := c.Phones(context.Background(), DeviceAuth{DeviceID: "x"}, "all"); return err },
		"resolve": func() error {
			return c.ResolvePhone(context.Background(), DeviceAuth{Credential: "x"}, "rc_1", PhoneAdd, "")
		},
		"revoke": func() error { return c.RevokePhone(context.Background(), DeviceAuth{}, "rc_1") },
	} {
		if err := call(); err == nil {
			t.Errorf("%s: a call without a full credential was accepted", name)
		}
	}
	if got.path != "" {
		t.Fatalf("a request reached the server: %+v", got)
	}
}

func TestPhonesAreListedByStateAndResolvedOrRevokedByIdentity(t *testing.T) {
	var got seen
	c := fakeAccounts(t, 200, `{"controllers":[{"id":"rc_1","ordinal":1,"name":"iPhone Safari","nameSource":"auto","state":"pending","uaClass":"ios-safari","claimsId":null,"createdAt":"t","lastSeenAt":"t","revokedAt":null,"expiresAt":"t2","keyThumbprint":"abc","replaceCandidate":"rc_0"}]}`, &got)
	phones, err := c.Phones(context.Background(), auth, "pending")
	if err != nil || len(phones) != 1 || phones[0].ID != "rc_1" || phones[0].State != "pending" || phones[0].ReplaceCandidate == nil || *phones[0].ReplaceCandidate != "rc_0" {
		t.Fatalf("phones = %+v, %v", phones, err)
	}
	if got.path != "/host/controllers" || got.query != "state=pending" {
		t.Fatalf("list request = %s?%s", got.path, got.query)
	}
	if err := c.ResolvePhone(context.Background(), auth, "rc_1", PhoneReplace, "rc_0"); err != nil {
		t.Fatal(err)
	}
	if got.path != "/host/controllers/rc_1/resolve" || got.body["action"] != "replace" || got.body["replaceId"] != "rc_0" {
		t.Fatalf("resolve = %s %v", got.path, got.body)
	}
	if err := c.ResolvePhone(context.Background(), auth, "rc_1", PhoneAdd, "ignored"); err != nil {
		t.Fatal(err)
	}
	if _, has := got.body["replaceId"]; has {
		t.Fatalf("a replace id rode along with an add: %v", got.body)
	}
	if err := c.RevokePhone(context.Background(), auth, "rc_1"); err != nil || got.path != "/host/controllers/rc_1/revoke" {
		t.Fatalf("revoke = %s, %v", got.path, err)
	}
	for _, bad := range []string{"", "../x", "a/b", "a?b"} {
		if c.ResolvePhone(context.Background(), auth, bad, PhoneAdd, "") == nil || c.RevokePhone(context.Background(), auth, bad) == nil {
			t.Errorf("id %q was accepted", bad)
		}
	}
}

func TestADeviceCallFailsWithTheServersOwnCode(t *testing.T) {
	var got seen
	c := fakeAccounts(t, 401, `{"error":{"code":"invalid_device","message":"The device credential is invalid or revoked."}}`, &got)
	_, err := c.NotifyPhones(context.Background(), auth, PushInput, 1)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Code != "invalid_device" {
		t.Fatalf("err = %v, want the typed account error", err)
	}
}
