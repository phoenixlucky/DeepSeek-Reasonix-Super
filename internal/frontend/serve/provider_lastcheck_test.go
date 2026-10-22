package serve

import (
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
)

func resetLastChecks(t *testing.T) {
	t.Helper()
	lastChecks.Lock()
	lastChecks.by = map[string]providerLastCheck{}
	lastChecks.Unlock()
	t.Cleanup(func() {
		lastChecks.Lock()
		lastChecks.by = map[string]providerLastCheck{}
		lastChecks.Unlock()
	})
}

func listedExisting(t *testing.T, base string) providerView {
	t.Helper()
	for _, v := range readJSON[[]providerView](t, base, "/providers") {
		if v.Name == "existing" {
			return v
		}
	}
	t.Fatal("provider existing not listed")
	return providerView{}
}

// A list that has never asked says nothing; after a plain re-check it carries
// how that ended, typed, so a failed test leaves a trace the row can show.
func TestListCarriesHowTheLastPlainCheckEnded(t *testing.T) {
	resetLastChecks(t)
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	if got := listedExisting(t, srv.URL).LastCheck; got != nil {
		t.Fatalf("a provider nobody checked carries %+v", got)
	}
	resp := postProvider(t, srv.URL, "/providers/check", `{"name":"existing"}`)
	resp.Body.Close()
	got := listedExisting(t, srv.URL).LastCheck
	if got == nil || got.OK || got.Code != codeProbeUnreachable || got.At == 0 {
		t.Fatalf("last check = %+v, want a failure with the typed unreachable code", got)
	}
}

// A check of unsaved form values is about the form, not about what is saved,
// so it leaves the saved entry's record alone.
func TestACheckOfUnsavedFormValuesIsNotRecordedAgainstTheSavedEntry(t *testing.T) {
	resetLastChecks(t)
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers/check", `{"name":"existing","baseUrl":"http://127.0.0.1:1/v1"}`)
	resp.Body.Close()
	if got := listedExisting(t, srv.URL).LastCheck; got != nil {
		t.Fatalf("a draft check was recorded against the saved entry: %+v", got)
	}
}

// Editing what the verdict was about drops it: a stale green dot is worse than
// none.
func TestAnEditedEntryDropsItsLastCheck(t *testing.T) {
	resetLastChecks(t)
	entry := &config.ProviderEntry{Name: "x", Kind: "openai", BaseURL: "https://a.example/v1", APIKeyEnv: "X_KEY"}
	noteLastCheck(entry, providerCheck{OK: true})
	if lastCheckOf(entry) == nil {
		t.Fatal("the recorded check was not found for the same entry")
	}
	moved := *entry
	moved.BaseURL = "https://b.example/v1"
	if got := lastCheckOf(&moved); got != nil {
		t.Fatalf("a changed address kept the old verdict: %+v", got)
	}
	rekeyed := *entry
	rekeyed.APIKeyEnv = "OTHER_KEY"
	if got := lastCheckOf(&rekeyed); got != nil {
		t.Fatalf("a changed key slot kept the old verdict: %+v", got)
	}
}
