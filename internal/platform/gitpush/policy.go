package gitpush

import (
	"errors"
	"net/url"
	"strings"
)

// ErrProtocolNotAllowed is a remote whose transport is not one a push may use.
var ErrProtocolNotAllowed = errors.New("gitpush: the remote's protocol is not allowed")

// allowedProtocols is what GIT_ALLOW_PROTOCOL carries: git itself refuses any
// other transport, remote helpers (ext::, fd::) and local paths included.
var allowedProtocols = "https:ssh:git"

// allowedSchemes is the same list for the check made before anything runs.
var allowedSchemes = map[string]bool{"https": true, "ssh": true, "git": true}

// Destination is where a remote URL leads, read by structure.
type Destination struct {
	Scheme string // https, ssh or git
	Host   string
	// Display is the URL without a password or token a person may have put in it.
	Display string
}

// Classify reads a remote URL. A URL with a scheme is parsed as one; the scp
// spelling (host:path, no scheme) is ssh; anything else, a local path or a
// helper transport included, is ErrProtocolNotAllowed. A password in the URL is
// dropped from Display, and a leading dash is never a destination.
func Classify(raw string) (Destination, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, "\x00\r\n") {
		return Destination{}, ErrProtocolNotAllowed
	}
	if scheme, _, found := strings.Cut(raw, "://"); found {
		if !allowedSchemes[strings.ToLower(scheme)] {
			return Destination{}, ErrProtocolNotAllowed
		}
		u, err := url.Parse(raw)
		if err != nil || strings.ToLower(scheme) != "file" && (u.Host == "" || u.Hostname() == "") {
			return Destination{}, ErrProtocolNotAllowed
		}
		display := *u
		if display.User != nil {
			display.User = url.User(display.User.Username())
		}
		return Destination{Scheme: strings.ToLower(scheme), Host: u.Hostname(), Display: display.String()}, nil
	}
	if host, ok := scpHost(raw); ok {
		return Destination{Scheme: "ssh", Host: host, Display: raw}, nil
	}
	return Destination{}, ErrProtocolNotAllowed
}

// scpHost is the host of [user@]host:path. The colon must come before any
// slash, or the text is a path, and a helper spelling "name::rest" is not an
// address at all.
func scpHost(raw string) (string, bool) {
	colon := strings.IndexByte(raw, ':')
	if colon <= 0 || strings.IndexByte(raw, '/') != -1 && strings.IndexByte(raw, '/') < colon {
		return "", false
	}
	if strings.HasPrefix(raw[colon:], "::") {
		return "", false
	}
	host := raw[:colon]
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.ContainsAny(host, " \t\\") || len(host) == 1 && (host[0] >= 'a' && host[0] <= 'z' || host[0] >= 'A' && host[0] <= 'Z') {
		return "", false
	}
	return host, true
}
