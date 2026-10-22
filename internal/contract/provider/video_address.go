package provider

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"sync"
)

// Codes for why an address was not sent, said to the model beside its name.
const (
	VideoURLUnavailable = "video.url_unavailable"
	VideoAddressOnly    = "video.native_address_only"
	VideoURLUnsupported = "video.url_unsupported"
)

// MaxVideoAddress bounds an address the model may name.
const MaxVideoAddress = 2048

var errVideoAddress = errors.New("not a public https address")

// ValidVideoAddress says whether raw is an address the vendor may be asked to
// visit: https, no credentials, and a DNS name that is not a single label,
// localhost, an IP literal or a name only a local network resolves. The host
// does not fetch it; the vendor does, so this only keeps an address that cannot
// be public, or that carries a secret, out of the request.
func ValidVideoAddress(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > MaxVideoAddress || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return errVideoAddress
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if _, err := netip.ParseAddr(host); err == nil || !strings.Contains(host, ".") {
		return errVideoAddress
	}
	for _, local := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa", ".intranet", ".corp"} {
		if host == strings.TrimPrefix(local, ".") || strings.HasSuffix(host, local) {
			return errVideoAddress
		}
	}
	return nil
}

// VideoAddressAccepted says whether a protocol takes this address as video: the
// Zhipu protocol any valid address, the Gemini protocols only the YouTube
// addresses Google documents for video (video-understanding page, read
// 2026-10-17). Every other protocol takes none.
func VideoAddressAccepted(protocol, raw string) bool {
	if ValidVideoAddress(raw) != nil {
		return false
	}
	switch protocol {
	case VideoProtocolZhipuURL:
		return true
	case VideoProtocolGeminiInline, VideoProtocolGeminiFiles:
		u, _ := url.Parse(raw)
		switch strings.ToLower(u.Hostname()) {
		case "youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be":
			return true
		}
	}
	return false
}

// VideoProtocolTakesFiles says whether a protocol can send a local file.
func VideoProtocolTakesFiles(protocol string) bool { return protocol != VideoProtocolZhipuURL }

var videoAddressGrants = struct {
	mu   sync.Mutex
	seen map[string]bool
}{seen: map[string]bool{}}

// GrantVideoAddress records, in this process only, an address a tool checked
// against the user's own words. Whatever sends a VideoRef later sends only an
// address granted here: a transcript can be edited, restored or imported with an
// address no tool ever saw, and the vendor would visit it.
func GrantVideoAddress(raw string) {
	videoAddressGrants.mu.Lock()
	defer videoAddressGrants.mu.Unlock()
	videoAddressGrants.seen[raw] = true
}

// VideoAddressGranted reports whether a tool in this process granted the address.
func VideoAddressGranted(raw string) bool {
	videoAddressGrants.mu.Lock()
	defer videoAddressGrants.mu.Unlock()
	return videoAddressGrants.seen[raw]
}
