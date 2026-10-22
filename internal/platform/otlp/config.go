package otlp

import (
	"errors"
	"net"
	"strings"

	"reasonix/internal/contract/config"
)

// Settings is everything the exporter needs, already resolved: the secret is a
// value here, read from the environment variable the config names.
type Settings struct {
	Endpoint    string
	HeaderName  string
	HeaderValue string
	ServiceName string
	Version     string
}

var (
	ErrEndpointInvalid     = config.ErrOTLPEndpointInvalid
	ErrEndpointCredentials = config.ErrOTLPEndpointCredentials
	ErrHeaderInvalid       = config.ErrOTLPHeaderInvalid
	ErrSecretOverPlainHTTP = errors.New("otlp header value would travel over plain http to a non-loopback host")
)

// target validates the settings and returns the URL spans are posted to.
func (s Settings) target() (string, error) {
	u, err := config.ParseOTLPEndpoint(s.Endpoint)
	if err != nil {
		return "", err
	}
	if s.HeaderName != "" && !config.OTLPHeaderNameOK(s.HeaderName) {
		return "", ErrHeaderInvalid
	}
	if s.HeaderValue != "" && u.Scheme == "http" && !loopback(u.Hostname()) {
		return "", ErrSecretOverPlainHTTP
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/v1/traces") {
		path += "/v1/traces"
	}
	u.Path = path
	return u.String(), nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
