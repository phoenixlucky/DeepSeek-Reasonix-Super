package config

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// WebAccessSettings is where web_search and web_fetch send what the user and the
// model ask for. Every field is user file only: a project file cannot choose a
// destination for them or agree to a charge on the user's behalf.
type WebAccessSettings struct {
	// WebSearchBackends is the ordered chain web_search tries, free backends
	// first. Empty keeps the model-side search.
	WebSearchBackends []string `toml:"web_search_backends"`
	// SearxngURL is the user's own SearXNG instance, with its JSON format on.
	SearxngURL string `toml:"searxng_url"`
	// WebSearchPaidAllowed names the paid search backends the user already agreed
	// to be charged for, so the card is not shown again.
	WebSearchPaidAllowed []string `toml:"web_search_paid_allowed"`
	// WebFetchBackends is the ordered chain web_fetch tries: "local" is the free
	// fetch from this machine, "deepseek_api" the paid one. Empty keeps the
	// local fetch alone.
	WebFetchBackends []string `toml:"web_fetch_backends"`
	// WebFetchPaidAllowed is WebSearchPaidAllowed for fetching.
	WebFetchPaidAllowed []string `toml:"web_fetch_paid_allowed"`
}

// any reports whether the user set anything here.
func (w WebAccessSettings) any() bool {
	return len(w.WebSearchBackends) > 0 || w.SearxngURL != "" || len(w.WebSearchPaidAllowed) > 0 ||
		len(w.WebFetchBackends) > 0 || len(w.WebFetchPaidAllowed) > 0
}

// The destinations a web backend list may name. A name outside these cannot be
// built, so a saved list never carries one the next session would refuse.
const (
	WebBackendLocal       = "local"
	WebBackendSearxng     = "searxng"
	WebBackendDeepSeekAPI = "deepseek_api"
)

// KnownSearchBackends lists the backends web_search can chain.
func KnownSearchBackends() []string { return []string{WebBackendSearxng, WebBackendDeepSeekAPI} }

// KnownFetchBackends lists the backends web_fetch can chain.
func KnownFetchBackends() []string { return []string{WebBackendLocal, WebBackendDeepSeekAPI} }

// PaidWebBackends lists the backends that bill per request, the only names a
// paid-allowed list can hold.
func PaidWebBackends() []string { return []string{WebBackendDeepSeekAPI} }

// ErrBadSearxngURL means the address is not one a search can be sent to.
var ErrBadSearxngURL = errors.New("searxng_url must be an http(s) address with a host, without credentials, query or fragment")

// ValidateSearxngURL accepts an http(s) origin with an optional path prefix and
// nothing that could redirect a query elsewhere.
func ValidateSearxngURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ErrBadSearxngURL
	}
	return nil
}

// UnknownWebBackendError names the entry a list may not hold.
type UnknownWebBackendError struct{ Name string }

func (e *UnknownWebBackendError) Error() string {
	return "unknown web backend " + strconv.Quote(e.Name)
}

// NormalizeWebBackends drops repeats, keeps order and refuses any name outside
// allowed. An empty list is valid: it keeps the built-in behaviour.
func NormalizeWebBackends(list, allowed []string) ([]string, error) {
	out := make([]string, 0, len(list))
	for _, name := range list {
		name = strings.TrimSpace(name)
		if !slices.Contains(allowed, name) {
			return nil, &UnknownWebBackendError{Name: name}
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out, nil
}
