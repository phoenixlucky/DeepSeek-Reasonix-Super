package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type keyCatcher struct {
	mu   sync.Mutex
	hits int
	auth []string
}

func (k *keyCatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	k.mu.Lock()
	k.hits++
	k.auth = append(k.auth, r.Header.Get("Authorization")+r.Header.Get("x-api-key"))
	k.mu.Unlock()
	w.WriteHeader(http.StatusUnauthorized)
}

func (k *keyCatcher) seen() (int, string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.hits, strings.Join(k.auth, "|")
}

func keyOriginServer(t *testing.T) (*httptest.Server, *keyCatcher, string) {
	t.Helper()
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	t.Cleanup(srv.Close)
	first := postProvider(t, srv.URL, "/providers", `{
		"name":"acme","kind":"openai","baseUrl":"https://api.acme.test/v1",
		"apiKey":"sk-saved-secret-1234","models":["m1"],"default":"m1"
	}`)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("save = %d", first.StatusCode)
	}
	catcher := &keyCatcher{}
	other := httptest.NewServer(catcher)
	t.Cleanup(other.Close)
	return srv, catcher, other.URL
}

func TestADraftAddressOnAnotherOriginNeverGetsTheSavedKey(t *testing.T) {
	srv, catcher, other := keyOriginServer(t)
	for _, c := range []struct{ path, body string }{
		{"/providers/check", `{"name":"acme","baseUrl":"` + other + `"}`},
		{"/providers/check/model", `{"name":"acme","model":"m1","baseUrl":"` + other + `"}`},
	} {
		resp := postProvider(t, srv.URL, c.path, c.body)
		raw := bodyText(t, resp)
		if resp.StatusCode != http.StatusConflict || !strings.Contains(raw, "provider.key_origin_changed") {
			t.Fatalf("%s = %d %s, want 409 provider.key_origin_changed", c.path, resp.StatusCode, raw)
		}
	}
	if hits, auth := catcher.seen(); hits != 0 {
		t.Fatalf("the other origin received %d request(s) carrying %q", hits, auth)
	}
}

func TestADraftWithItsOwnKeyOrTheSavedOriginIsNotRefused(t *testing.T) {
	srv, catcher, other := keyOriginServer(t)
	resp := postProvider(t, srv.URL, "/providers/check", `{"name":"acme","baseUrl":"`+other+`","apiKey":"sk-typed-now-5678"}`)
	raw := bodyText(t, resp)
	if strings.Contains(raw, "provider.key_origin_changed") {
		t.Fatalf("a typed key was refused: %s", raw)
	}
	if _, auth := catcher.seen(); strings.Contains(auth, "sk-saved-secret-1234") {
		t.Fatalf("the saved key reached the other origin: %q", auth)
	}
	same := postProvider(t, srv.URL, "/providers/check", `{"name":"acme","baseUrl":"https://api.acme.test/other-path"}`)
	if got := bodyText(t, same); strings.Contains(got, "provider.key_origin_changed") {
		t.Fatalf("the same origin was refused: %s", got)
	}
}

func bodyText(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	text, err := readAll(resp)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
