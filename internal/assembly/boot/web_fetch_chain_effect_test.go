package boot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

type fetchRun struct {
	probe   *capabilityProbeProvider
	paid    chan *http.Request
	hosts   chan string
	usage   func() []event.Event
	notices func() []event.Event
}

// fetchChainSession runs one model turn that fetches url, with the user's
// config, through the real Build. The request that would leave for
// api.deepseek.com is captured, never sent.
func fetchChainSession(t *testing.T, url, userConfig string) fetchRun {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	writeFile(t, filepath.Dir(config.UserConfigPath()), ".env", "DS_FETCH_FIXTURE_KEY=sk-fixture\n")
	paid := make(chan *http.Request, 4)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paid <- r.Clone(context.Background())
		_, _ = fmt.Fprint(w, `{"title":"Remote page","content":"# Remote page\n\ncontent only DeepSeek could read"}`)
	}))
	t.Cleanup(api.Close)
	hosts := make(chan string, 4)
	searchTransportOverride = rewriteToServer{target: api, hosts: hosts}
	t.Cleanup(func() { searchTransportOverride = nil })
	probe := &capabilityProbeProvider{calls: []string{fmt.Sprintf(`{"action":"call","capability_id":"tool:web_fetch","arguments":{"url":%q}}`, url)}}
	kind := "fetch-chain-" + t.Name()
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return probe, nil })
	writeFile(t, filepath.Dir(config.UserConfigPath()), "config.toml", fmt.Sprintf(`default_model = "chat/main"
[agent]
system_prompt = "BASE"
%s
[[providers]]
name = "chat"
kind = %q
model = "main"
[[providers]]
name = "ds"
kind = "openai"
base_url = "https://api.deepseek.com"
model = "deepseek-flash"
api_key_env = "DS_FETCH_FIXTURE_KEY"
`, userConfig, kind))
	var mu sync.Mutex
	var usage, notices []event.Event
	sink := event.FuncSink(func(e event.Event) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case e.Kind == event.Usage && e.UsageSource == event.UsageSourceDeepSeekFetch:
			usage = append(usage, e)
		case e.Code == event.NoticeCodeWebFetchBackendFallback:
			notices = append(notices, e)
		}
	})
	ctrl, err := Build(context.Background(), Options{Sink: sink, Model: "chat/main", WorkspaceRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	if err := ctrl.Run(context.Background(), "read that page"); err != nil {
		t.Fatal(err)
	}
	return fetchRun{probe: probe, paid: paid, hosts: hosts,
		usage:   func() []event.Event { mu.Lock(); defer mu.Unlock(); return append([]event.Event(nil), usage...) },
		notices: func() []event.Event { mu.Lock(); defer mu.Unlock(); return append([]event.Event(nil), notices...) }}
}

const unreachableHere = "https://blocked-locally.zzzzqq/docs/page"

// With the chain set but the local fetch answering, nothing is sent to DeepSeek
// and the model sees the page exactly as web_fetch has always shown it.
func TestEffectFetchChainKeepsTheLocalAnswerAndCallsNobodyElse(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("local page text"))
	}))
	defer site.Close()
	run := fetchChainSession(t, site.URL, "[tools]\nweb_fetch_backends = [\"local\", \"deepseek_api\"]\nweb_fetch_paid_allowed = [\"deepseek_api\"]\n")
	if len(run.paid) != 0 {
		t.Fatalf("DeepSeek was called %d times although the local fetch answered", len(run.paid))
	}
	results := run.probe.toolResults()
	if len(results) != 1 || !strings.Contains(results[0], "status 200 OK · text/plain · 15 bytes\n\nlocal page text") {
		t.Fatalf("tool results = %v", results)
	}
}

// The paid fetch is not called until the user has agreed, and the model is told
// why in a typed refusal.
func TestEffectPaidFetchIsNotCalledWithoutConsent(t *testing.T) {
	run := fetchChainSession(t, unreachableHere, "[tools]\nweb_fetch_backends = [\"local\", \"deepseek_api\"]\n")
	if len(run.paid) != 0 {
		t.Fatalf("DeepSeek was called %d times without consent", len(run.paid))
	}
	results := run.probe.toolResults()
	if len(results) != 1 || !strings.Contains(results[0], "fetch.free_failed_paid_needs_consent") {
		t.Fatalf("tool results = %v", results)
	}
	if n := run.notices(); len(n) != 1 || !strings.HasPrefix(n[0].Detail, "local|transport|0||deepseek_api") {
		t.Fatalf("notices = %+v", n)
	}
}

// With the user's standing agreement the paid fetch goes out as documented, the
// model is told the page was paid for, and the charge lands in the usage view.
func TestEffectPaidFetchWithStandingConsentIsBilledOncePerSuccess(t *testing.T) {
	run := fetchChainSession(t, unreachableHere, "[tools]\nweb_fetch_backends = [\"local\", \"deepseek_api\"]\nweb_fetch_paid_allowed = [\"deepseek_api\"]\n")
	if len(run.paid) != 1 {
		t.Fatalf("DeepSeek saw %d requests, want 1", len(run.paid))
	}
	req := <-run.paid
	if h := <-run.hosts; h != "api.deepseek.com" {
		t.Fatalf("addressed to %q, want api.deepseek.com", h)
	}
	if req.Method != http.MethodPost || req.URL.Path != "/v1/tools/fetch" || req.Header.Get("Authorization") != "Bearer sk-fixture" {
		t.Fatalf("request = %s %s auth=%q", req.Method, req.URL.Path, req.Header.Get("Authorization"))
	}
	results := run.probe.toolResults()
	if len(results) != 1 || !strings.Contains(results[0], "fetched by deepseek_api · billed") || !strings.Contains(results[0], "content only DeepSeek could read") {
		t.Fatalf("tool results = %v", results)
	}
	if u := run.usage(); len(u) != 1 || u[0].CostQuote == nil || u[0].CostQuote.Original.Amount != "0.00075" {
		t.Fatalf("usage = %+v, want one request at 0.00075", u)
	}
}

// An address on this machine's own network is never sent to a third party, even
// with the user's standing agreement, and the model is told it is a refusal.
func TestEffectPaidFetchNeverSendsAnInternalAddress(t *testing.T) {
	run := fetchChainSession(t, "http://169.254.169.254/latest/meta-data/", "[tools]\nweb_fetch_backends = [\"local\", \"deepseek_api\"]\nweb_fetch_paid_allowed = [\"deepseek_api\"]\n")
	if len(run.paid) != 0 {
		t.Fatalf("an internal address was sent to DeepSeek %d times", len(run.paid))
	}
	results := run.probe.toolResults()
	if len(results) != 1 || !strings.Contains(results[0], "fetch.internal_address") {
		t.Fatalf("tool results = %v", results)
	}
}

// The local fetch fails the ordinary way here (nothing listens), so only the
// address check stands between this machine's own network and a third party.
func TestEffectPaidFetchVetsAnAddressTheLocalFetchMerelyFailedOn(t *testing.T) {
	run := fetchChainSession(t, "http://127.0.0.1:9/private-dev-server", "[tools]\nweb_fetch_backends = [\"local\", \"deepseek_api\"]\nweb_fetch_paid_allowed = [\"deepseek_api\"]\n")
	if len(run.paid) != 0 {
		t.Fatalf("a loopback address was sent to DeepSeek %d times", len(run.paid))
	}
	results := run.probe.toolResults()
	if len(results) != 1 || !strings.Contains(results[0], "fetch.internal_address") {
		t.Fatalf("tool results = %v", results)
	}
}
