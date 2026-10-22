package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/account"
	"reasonix/internal/platform/remotecloud"
)

type fakePhones struct {
	list     []account.Phone
	err      error
	resolved []string
	revoked  []string
}

func (f *fakePhones) Phones(context.Context, string) ([]account.Phone, error) { return f.list, f.err }

func (f *fakePhones) ResolvePhone(_ context.Context, id, action, replaceID string) error {
	f.resolved = append(f.resolved, id+":"+action+":"+replaceID)
	return f.err
}

func (f *fakePhones) RevokePhone(_ context.Context, id string) error {
	f.revoked = append(f.revoked, id)
	return f.err
}

func phoneServer(t *testing.T, src CloudPhones) *httptest.Server {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	share := NewDeviceShare(nil)
	h := NewHub(HubOptions{Share: share})
	if src != nil {
		h.SetCloudPhones(src)
	}
	srv := httptest.NewServer(operatorHandler(h))
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestPhonesAreListedWithTheWaitingOnesAndWithoutTheRevokedOnes(t *testing.T) {
	src := &fakePhones{list: []account.Phone{
		{ID: "rc_1", Ordinal: new(1), Name: "iPhone Safari", State: "active", UAClass: "ios-safari", CreatedAt: "a", LastSeenAt: "b", KeyThumbprint: "t1"},
		{ID: "rc_2", Name: "Android Chrome", State: "pending", ExpiresAt: new("later"), KeyThumbprint: "t2", ReplaceCandidate: new("rc_1")},
		{ID: "rc_3", Ordinal: new(2), State: "revoked"},
	}}
	srv := phoneServer(t, src)
	status, body := call(t, srv, http.MethodGet, "/share/phones", "")
	phones, _ := body["phones"].([]any)
	if status != 200 || len(phones) != 2 {
		t.Fatalf("list = %d %v, want the two that are not revoked", status, body)
	}
	pending := phones[1].(map[string]any)
	if pending["id"] != "rc_2" || pending["state"] != "pending" || pending["replaceCandidate"] != "rc_1" || pending["expiresAt"] != "later" {
		t.Fatalf("the waiting device = %v", pending)
	}
	if _, has := phones[0].(map[string]any)["endpoint"]; has {
		t.Fatal("the list carries an address")
	}
}

func TestAWaitingPhoneIsAddedReplacedOrRejectedByIdentityAndTheListComesBackFresh(t *testing.T) {
	src := &fakePhones{list: []account.Phone{{ID: "rc_2", State: "pending"}}}
	srv := phoneServer(t, src)
	for _, c := range []struct{ body, want string }{
		{`{"id":"rc_2","action":"add"}`, "rc_2:add:"},
		{`{"id":"rc_2","action":"replace","replaceId":"rc_1"}`, "rc_2:replace:rc_1"},
		{`{"id":"rc_2","action":"reject"}`, "rc_2:reject:"},
	} {
		status, body := call(t, srv, http.MethodPost, "/share/phones/resolve", c.body)
		if status != 200 || len(body["phones"].([]any)) != 1 {
			t.Fatalf("%s = %d %v", c.body, status, body)
		}
		if got := src.resolved[len(src.resolved)-1]; got != c.want {
			t.Fatalf("resolved %q, want %q", got, c.want)
		}
	}
	for _, bad := range []string{`{"action":"add"}`, `{"id":"rc_2"}`, `{"id":"rc_2","action":"approve"}`, `not json`} {
		if status, _ := call(t, srv, http.MethodPost, "/share/phones/resolve", bad); status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Errorf("%s was accepted: %d", bad, status)
		}
	}
	if len(src.resolved) != 3 {
		t.Fatalf("a malformed request reached the account service: %v", src.resolved)
	}
	if status, _ := call(t, srv, http.MethodPost, "/share/phones/revoke", `{"id":"rc_2"}`); status != 200 || len(src.revoked) != 1 || src.revoked[0] != "rc_2" {
		t.Fatalf("revoke = %d %v", status, src.revoked)
	}
}

func TestWhyThePhonesCouldNotBeReadIsAnAnswerByIdentity(t *testing.T) {
	for name, c := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"not registered": {remotecloud.ErrNotRegistered, http.StatusConflict, "phones.not_registered"},
		"not found":      {&account.Error{Status: 404, Code: "controller_not_found"}, http.StatusNotFound, "phones.not_found"},
		"refused":        {&account.Error{Status: 409, Code: "not_pending"}, http.StatusConflict, "phones.refused"},
		"signed out":     {&account.Error{Status: 401, Code: "invalid_device"}, http.StatusConflict, "phones.refused"},
		"service down":   {errors.New("dial tcp: connection refused"), http.StatusBadGateway, "phones.account_unreachable"},
		"service error":  {&account.Error{Status: 500, Code: "internal"}, http.StatusBadGateway, "phones.account_unreachable"},
	} {
		srv := phoneServer(t, &fakePhones{err: c.err})
		status, body := call(t, srv, http.MethodGet, "/share/phones", "")
		if status != c.status || body["code"] != c.code {
			t.Errorf("%s: %d %v, want %d %s", name, status, body, c.status, c.code)
		}
	}
	srv := phoneServer(t, nil)
	if status, body := call(t, srv, http.MethodGet, "/share/phones", ""); status != http.StatusConflict || body["code"] != "phones.unavailable" {
		t.Fatalf("with no account connection: %d %v", status, body)
	}
}
