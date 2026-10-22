// Package walletformat is the closed set of shapes a provider's balance endpoint
// can answer in. A format is declared on a provider entry (balance_format) or
// implied by a vendor's own hostname, never inferred from a name or from what an
// answer happens to contain. The configuration and the reader of the answer both
// need the set, and neither may own it.
package walletformat

import (
	"net/url"
	"slices"
	"strings"
)

// Format names one vendor's balance response shape.
type Format string

const (
	DeepSeek   Format = "deepseek"
	Moonshot   Format = "moonshot"
	OpenRouter Format = "openrouter"
)

// All is every declarable format, in the order a client lists them.
func All() []Format { return []Format{DeepSeek, Moonshot, OpenRouter} }

// Valid reports whether f is a declared format.
func (f Format) Valid() bool { return slices.Contains(All(), f) }

// List is the declared formats as one comma-separated string, for messages.
func List() string {
	names := make([]string, 0, len(All()))
	for _, f := range All() {
		names = append(names, string(f))
	}
	return strings.Join(names, ", ")
}

// hosts maps a vendor's own API hostname to the format it answers in. Host
// equality is the test, the same as pricing.OfficialProviderForEndpoint: only the
// vendor's own endpoint is known to answer in its own shape, and an entry merely
// named after a vendor is not evidence of that endpoint.
var hosts = map[string]Format{
	"api.deepseek.com": DeepSeek,
	"api.moonshot.cn":  Moonshot,
	"api.moonshot.ai":  Moonshot,
	"openrouter.ai":    OpenRouter,
}

// Host is the lower-cased hostname of an endpoint, empty when it has none.
func Host(endpoint string) string {
	if u, err := url.Parse(strings.TrimSpace(endpoint)); err == nil {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// Implied is the format a vendor's own hostname answers in, empty for any other
// host: a relay on its own domain must declare one.
func Implied(endpoint string) Format { return hosts[Host(endpoint)] }
