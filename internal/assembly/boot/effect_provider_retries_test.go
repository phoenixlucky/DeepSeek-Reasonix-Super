package boot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

type flakyWireProvider struct {
	attempts atomic.Int32
}

func (p *flakyWireProvider) Name() string { return "boot-flaky-wire" }

func (p *flakyWireProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	cl := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		p.attempts.Add(1)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("busy")), Header: http.Header{"Retry-After": {"1"}}}, nil
	})}
	_, err := provider.SendWithRetry(ctx, cl, provider.SendOptions{Provider: "boot-flaky-wire"}, func(c context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(c, http.MethodPost, "http://provider.invalid/v1", nil)
	})
	return nil, err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type retryRun struct {
	prov    *flakyWireProvider
	notices []event.Event
	err     error
}

func runFlakyTurn(t *testing.T, userConfig string, stopAfter int) retryRun {
	t.Helper()
	home := statedBootHome(t)
	t.Setenv("REASONIX_HOME", home)
	dir := testenv.TempDir(t)
	prov := &flakyWireProvider{}
	kind := fmt.Sprintf("boot-flaky-%s", strings.ReplaceAll(t.Name(), "/", "-"))
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return prov, nil })
	writeFile(t, dir, "reasonix.toml", "default_model = \"test-model\"\n\n[codegraph]\nenabled = false\n\n[[providers]]\nname = \"test-model\"\nkind = \""+kind+"\"\nmodel = \"x\"\n")
	writeFile(t, home, "config.toml", userConfig)
	approveWorkspaceAt(t, home, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var mu sync.Mutex
	out := retryRun{prov: prov}
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind != event.Retrying || e.RetryScope != event.RetryScopeHeaders {
			return
		}
		mu.Lock()
		out.notices = append(out.notices, e)
		n := len(out.notices)
		mu.Unlock()
		if stopAfter > 0 && n >= stopAfter {
			cancel()
		}
	})
	ctrl, err := Build(ctx, Options{Home: home, WorkspaceRoot: dir, Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctrl.Close)
	out.err = ctrl.Run(ctx, "hi")
	mu.Lock()
	defer mu.Unlock()
	return out
}

// The user's retry setting reaches the provider call through the real
// assembly: a limit of one means two attempts and a notice that says so.
func TestEffectProviderRetryLimitReachesTheProviderCall(t *testing.T) {
	got := runFlakyTurn(t, "[agent]\nmax_provider_retries = 1\n", 0)
	if n := got.prov.attempts.Load(); n != 2 {
		t.Fatalf("%d provider attempts with max_provider_retries = 1, want 2", n)
	}
	if len(got.notices) != 1 || got.notices[0].RetryMax != 1 || got.notices[0].RetryAttempt != 1 {
		t.Fatalf("retry notices = %+v, want one attempt 1 of max 1", got.notices)
	}
}

// Zero removes the limit: the run is still retrying past the point a limit of
// one would have stopped it, and the notices carry no maximum to count against.
func TestEffectProviderRetriesWithoutALimitKeepGoing(t *testing.T) {
	got := runFlakyTurn(t, "[agent]\nmax_provider_retries = 0\n", 3)
	if n := got.prov.attempts.Load(); n < 3 {
		t.Fatalf("%d provider attempts with the limit removed, want at least 3", n)
	}
	for _, e := range got.notices {
		if e.RetryMax != 0 {
			t.Fatalf("a notice names a maximum of %d for an unlimited run", e.RetryMax)
		}
	}
}
