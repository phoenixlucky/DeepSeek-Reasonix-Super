package boot

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"reasonix/internal/base/netclient"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/builtin"
	"reasonix/internal/tools/fetchchain"
)

const deepseekFetchTimeout = 60 * time.Second

// addFetchChain registers web_fetch over the user's ordered backends, replacing
// the plain local fetch. A backend that cannot be built is reported and left out;
// with none left the built-in fetch stays as it was.
func addFetchChain(reg *tool.Registry, cfg *config.Config, proxy netclient.ProxySpec, sink event.Sink) {
	if len(cfg.Tools.WebFetchBackends) == 0 || cfg.Environment.Offline ||
		(len(cfg.Tools.Enabled) > 0 && !slices.Contains(cfg.Tools.Enabled, "web_fetch")) {
		return
	}
	var backends []fetchchain.Backend
	for _, name := range cfg.Tools.WebFetchBackends {
		backend, err := buildFetchBackend(name, cfg, proxy)
		if err != nil {
			report(sink, event.Event{Level: event.LevelWarn, Code: event.NoticeCodeWebFetchBackendUnusable,
				Text: "A configured web fetch backend cannot be used and was skipped: " + err.Error(), Detail: name})
			continue
		}
		backends = append(backends, backend)
	}
	if len(backends) == 0 {
		return
	}
	allowed := slices.Clone(cfg.Tools.WebFetchPaidAllowed)
	reg.Add(&fetchchain.Chain{
		Backends: backends,
		Vet:      builtin.VetFetchURL,
		Consent: func(ctx context.Context, b fetchchain.Backend, url string) bool {
			if slices.Contains(allowed, b.Name()) {
				return true
			}
			approver, ok := tool.PaidSearchApproverFrom(ctx)
			if !ok {
				return false
			}
			allow, err := approver.ApprovePaidSearch(ctx, tool.PaidSearchRequest{
				Service: tool.PaidServiceFetch, Backend: b.Name(), Query: url, Price: paidPrice("fetch")})
			return err == nil && allow
		},
		OnFallback: func(f fetchchain.Fallback) {
			next := f.Next
			if next == "" {
				next = "none"
			}
			report(sink, event.Event{Level: event.LevelWarn, Code: event.NoticeCodeWebFetchBackendFallback,
				Text:   fmt.Sprintf("Web fetch backend %s did not answer (%s); trying %s.", f.Backend, f.Kind, next),
				Detail: fmt.Sprintf("%s|%s|%d|%s|%s", f.Backend, f.Kind, f.Status, f.Code, next)})
		},
		OnBilled: func(string) { reportPaidRequest(sink, "fetch") },
	})
}

func buildFetchBackend(name string, cfg *config.Config, proxy netclient.ProxySpec) (fetchchain.Backend, error) {
	switch name {
	case "local":
		return fetchchain.NewLocal(builtin.NewLocalFetcher(proxy)), nil
	case "deepseek_api":
		base, key, err := cfg.ResolveDeepSeekAPI()
		if err != nil {
			return nil, err
		}
		client, err := netclient.NewHTTPClient(proxy, netclient.TransportOptions{})
		if err != nil {
			return nil, err
		}
		client.Timeout = deepseekFetchTimeout
		if searchTransportOverride != nil {
			client.Transport = searchTransportOverride
		}
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return fetchchain.NewDeepSeekFetch(base, key, client), nil
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}
