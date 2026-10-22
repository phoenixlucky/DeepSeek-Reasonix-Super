package serve

import (
	"strings"
	"sync"
	"time"

	"reasonix/internal/contract/config"
)

// providerLastCheck is how the most recent plain check of a saved provider
// ended, for the list to show without asking again.
type providerLastCheck struct {
	At   int64  `json:"at"`
	OK   bool   `json:"ok"`
	Code string `json:"code,omitempty"`
}

// lastChecks lives for the process only: a green dot read from yesterday's run
// is worse than no dot. A record is keyed by what the check was about, so
// editing the address, protocol or key slot drops it rather than carrying a
// verdict about an endpoint that is no longer the one saved.
var lastChecks = struct {
	sync.Mutex
	by map[string]providerLastCheck
}{by: map[string]providerLastCheck{}}

func lastCheckKey(p *config.ProviderEntry) string {
	return strings.Join([]string{p.Name, strings.ToLower(strings.TrimSpace(p.Kind)), strings.TrimSpace(p.BaseURL), p.APIKeyEnv}, "\x00")
}

// noteLastCheck records the outcome of re-checking the saved entry. A check made
// with the form's unsaved values says nothing about what is saved, so the caller
// does not record those.
func noteLastCheck(p *config.ProviderEntry, got providerCheck) {
	lastChecks.Lock()
	defer lastChecks.Unlock()
	lastChecks.by[lastCheckKey(p)] = providerLastCheck{At: time.Now().UnixMilli(), OK: got.OK, Code: got.Code}
}

func lastCheckOf(p *config.ProviderEntry) *providerLastCheck {
	lastChecks.Lock()
	defer lastChecks.Unlock()
	if got, ok := lastChecks.by[lastCheckKey(p)]; ok {
		return &got
	}
	return nil
}
