package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestMachineSettingsAccountState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		token      string
		granted    bool
		identity   int
		status     int
		signedIn   bool
		expired    bool
		dependency bool
	}{
		{name: "signed in", token: "test-token", granted: true, identity: http.StatusOK, status: http.StatusOK, signedIn: true},
		{name: "signed out", granted: true, status: http.StatusOK},
		{name: "expired", token: "test-token", granted: true, identity: http.StatusUnauthorized, status: http.StatusOK, expired: true},
		{name: "identity unavailable", token: "test-token", granted: true, identity: http.StatusServiceUnavailable, status: http.StatusOK, signedIn: true, dependency: true},
		{name: "host grant required", token: "test-token", status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REASONIX_HOME", testenv.TempDir(t))
			t.Setenv("REASONIX_ACCOUNT_TOKEN", tc.token)
			identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/me" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected identity request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.identity)
				if tc.identity == http.StatusOK {
					_, _ = w.Write([]byte(`{"user":{"id":42,"handle":"contributor"}}`))
				} else {
					_, _ = w.Write([]byte(`{"error":"identity unavailable"}`))
				}
			}))
			defer identity.Close()
			t.Setenv("REASONIX_ACCOUNTS_URL", identity.URL)
			h := NewHub(HubOptions{})
			local := hubRuntime(t, h, testenv.TempDir(t))
			if tc.granted {
				local.Server.AllowAccountAuth()
			}
			w := httptest.NewRecorder()
			operatorHandler(h).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/host/settings/account", nil))
			if w.Code != tc.status {
				t.Fatalf("machine account = %d %s, want %d", w.Code, w.Body.String(), tc.status)
			}
			var got struct {
				SignedIn bool   `json:"signedIn"`
				Expired  bool   `json:"expired"`
				Error    string `json:"error"`
				Code     string `json:"code"`
				User     struct {
					Handle string `json:"handle"`
				} `json:"user"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.SignedIn != tc.signedIn || got.Expired != tc.expired {
				t.Fatalf("account state = %+v", got)
			}
			if tc.dependency && got.Error == "" {
				t.Fatal("identity failure must remain visible without signing the user out")
			}
			if tc.identity == http.StatusOK && got.User.Handle != "contributor" {
				t.Fatalf("signed-in identity = %+v", got.User)
			}
			if !tc.granted && got.Code != "account.signin_disabled" {
				t.Fatalf("host refusal = %q", got.Code)
			}
		})
	}
}
