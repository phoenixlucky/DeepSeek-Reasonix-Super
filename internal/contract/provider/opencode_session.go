package provider

import (
	"crypto/rand"
	"net/http"
	"net/url"
	"strings"
)

const openCodeSessionHeader = "x-opencode-session"

// NewOpenCodeSessionID mints the routing identity one client presents to
// OpenCode Go for as long as it lives.
func NewOpenCodeSessionID() string { return "reasonix-" + rand.Text() }

// openCodeGoPrefixes are the path prefixes OpenCode Go serves its gateway under:
// the original one and the one its documentation now names. A prefix is added
// here once the service is seen answering on it.
var openCodeGoPrefixes = []string{"/zen/go", "/inference/go"}

// IsOpenCodeGoEndpoint reports whether rawURL is on OpenCode Go's gateway. Zen
// and relays at other hosts do not match.
func IsOpenCodeGoEndpoint(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && isOpenCodeGoURL(u)
}

func isOpenCodeGoURL(u *url.URL) bool {
	if u == nil || !strings.EqualFold(u.Hostname(), "opencode.ai") {
		return false
	}
	path := strings.ToLower(u.Path)
	for _, prefix := range openCodeGoPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// ApplyOpenCodeGoIdentity sets the client identity OpenCode Go requires of a
// third-party client: without x-opencode-session the gateway answers 400.
// Values the user configured as custom headers are kept.
func ApplyOpenCodeGoIdentity(req *http.Request, sessionID string) {
	if req == nil || !isOpenCodeGoURL(req.URL) {
		return
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Reasonix")
	}
	if sessionID != "" && req.Header.Get(openCodeSessionHeader) == "" {
		req.Header.Set(openCodeSessionHeader, sessionID)
	}
}
