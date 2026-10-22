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
	"reasonix/internal/contract/pricing"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/websearch"
)

const (
	searxngTimeout        = 20 * time.Second
	deepseekSearchTimeout = 30 * time.Second
)

// addSearchChain registers web_search over the user's ordered backends. A
// backend that cannot be built is reported and left out; with none left the
// tool is not registered at all, so the model never sees a search that cannot run.
func addSearchChain(reg *tool.Registry, cfg *config.Config, proxy netclient.ProxySpec, sink event.Sink) {
	if cfg.Environment.Offline || (len(cfg.Tools.Enabled) > 0 && !slices.Contains(cfg.Tools.Enabled, "web_search")) {
		return
	}
	var backends []websearch.Backend
	for _, name := range cfg.Tools.WebSearchBackends {
		backend, err := buildSearchBackend(name, cfg, proxy)
		if err != nil {
			report(sink, event.Event{Level: event.LevelWarn, Code: event.NoticeCodeWebSearchBackendUnusable,
				Text: "A configured web search backend cannot be used and was skipped: " + err.Error(), Detail: name})
			continue
		}
		backends = append(backends, backend)
	}
	if len(backends) == 0 {
		return
	}
	allowed := slices.Clone(cfg.Tools.WebSearchPaidAllowed)
	reg.Add(&websearch.Chain{
		Backends: backends,
		Consent: func(ctx context.Context, b websearch.Backend, query string) bool {
			if slices.Contains(allowed, b.Name()) {
				return true
			}
			approver, ok := tool.PaidSearchApproverFrom(ctx)
			if !ok {
				return false
			}
			allow, err := approver.ApprovePaidSearch(ctx, tool.PaidSearchRequest{Backend: b.Name(), Query: query, Price: paidSearchPrice()})
			return err == nil && allow
		},
		OnFallback: func(f websearch.Fallback) {
			next := f.Next
			if next == "" {
				next = "none"
			}
			report(sink, event.Event{Level: event.LevelWarn, Code: event.NoticeCodeWebSearchBackendFallback,
				Text:   fmt.Sprintf("Web search backend %s did not answer (%s); trying %s.", f.Backend, f.Kind, next),
				Detail: fmt.Sprintf("%s|%s|%d|%s|%s", f.Backend, f.Kind, f.Status, f.Code, next)})
		},
		OnBilled: func(string) { reportPaidRequest(sink, "search") },
	})
}

// searchTransportOverride replaces the DeepSeek API client's transport. It is
// nil outside tests, which use it to see the request that would leave for
// api.deepseek.com without sending it there.
var searchTransportOverride http.RoundTripper

const (
	deepseekSearchModelRef = "deepseek-api/search"
	deepseekFetchModelRef  = "deepseek-api/fetch"
)

// paidSearchPrice is the price the consent card quotes, read from the price
// book and never written here.
func paidSearchPrice() string { return paidPrice("search") }

func paidPrice(service string) string {
	rate, ok := pricing.CurrentRequestRate("deepseek", service)
	if !ok {
		return "an unpublished price"
	}
	return fmt.Sprintf("%s %.2f per 1,000 successful requests (%s, read %s; DeepSeek announces the billing start date separately)",
		rate.Currency, rate.Per1000, rate.DocURL, rate.CheckedOn)
}

// reportPaidRequest records one successful paid request in the usage view, under
// its own source, at the price book's rate.
func reportPaidRequest(sink event.Sink, service string) {
	rate, ok := pricing.CurrentRequestRate("deepseek", service)
	if sink == nil || !ok {
		return
	}
	source, ref := event.UsageSourceDeepSeekSearch, deepseekSearchModelRef
	if service == "fetch" {
		source, ref = event.UsageSourceDeepSeekFetch, deepseekFetchModelRef
	}
	quote := pricing.RequestQuote(rate, 1, ref, source)
	sink.Emit(event.Event{Kind: event.Usage, ModelRef: ref, Usage: &provider.Usage{RequestCount: 1},
		CostQuote: &quote, UsageSource: source})
}

func buildSearchBackend(name string, cfg *config.Config, proxy netclient.ProxySpec) (websearch.Backend, error) {
	switch name {
	case "searxng":
		client, err := netclient.NewHTTPClient(proxy, netclient.TransportOptions{})
		if err != nil {
			return nil, err
		}
		client.Timeout = searxngTimeout
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return websearch.NewSearxng(cfg.Tools.SearxngURL, client)
	case "deepseek_api":
		base, key, err := cfg.ResolveDeepSeekAPI()
		if err != nil {
			return nil, err
		}
		client, err := netclient.NewHTTPClient(proxy, netclient.TransportOptions{})
		if err != nil {
			return nil, err
		}
		client.Timeout = deepseekSearchTimeout
		if searchTransportOverride != nil {
			client.Transport = searchTransportOverride
		}
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return websearch.NewDeepSeekAPI(base, key, client), nil
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}
