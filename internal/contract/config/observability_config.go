package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ObservabilityConfig governs what leaves the machine for the user's own
// observability stack. Everything here is user-file only.
type ObservabilityConfig struct {
	OTLP OTLPConfig `toml:"otlp"`
}

// OTLPConfig is the opt-in trace export. Nothing is sent unless Enabled is set
// and Endpoint names a collector. The credential is never in the file: HeaderEnv
// names the environment variable that holds the header's value.
type OTLPConfig struct {
	Enabled     bool   `toml:"enabled"`
	Endpoint    string `toml:"endpoint"`
	HeaderName  string `toml:"header_name"`
	HeaderEnv   string `toml:"header_env"`
	ServiceName string `toml:"service_name"`
}

// otlpHeaderEnv admits only variables made for this: a name like AWS_SECRET_ACCESS_KEY
// must never be readable by pointing a setting at it.
var otlpHeaderEnv = regexp.MustCompile(`^(OTEL|REASONIX_OTEL)_[A-Z0-9_]{1,64}$`)

// OTLPHeaderEnvOK reports whether name may be read as the export credential.
func OTLPHeaderEnvOK(name string) bool { return name == "" || otlpHeaderEnv.MatchString(name) }

// OTLPActive says the export is on: switched on and given somewhere to go.
func (c *Config) OTLPActive() bool {
	return c != nil && c.Observability.OTLP.Enabled && strings.TrimSpace(c.Observability.OTLP.Endpoint) != ""
}

func (h heldScope) narrowObservability(c *Config) {
	if c.Observability != h.observability {
		c.ignoreProject("observability.otlp", fmt.Sprintf("enabled=%t endpoint=%q", c.Observability.OTLP.Enabled, c.Observability.OTLP.Endpoint), ProjectUserOnly)
	}
	c.Observability = h.observability
}

func renderObservabilitySection(b *strings.Builder, c *Config) {
	o := c.Observability.OTLP
	if o == (OTLPConfig{}) {
		return
	}
	b.WriteString("[observability.otlp]\n")
	fmt.Fprintf(b, "enabled = %v   # export each turn as OpenTelemetry GenAI traces; structure, timing, counts and names only, never prompts, arguments, results or paths\n", o.Enabled)
	fmt.Fprintf(b, "endpoint = %q   # OTLP/HTTP collector, for example http://localhost:4318\n", o.Endpoint)
	if o.HeaderName != "" {
		fmt.Fprintf(b, "header_name = %q   # the header that carries your credential\n", o.HeaderName)
	}
	if o.HeaderEnv != "" {
		fmt.Fprintf(b, "header_env = %q   # the OTEL_* or REASONIX_OTEL_* environment variable holding the header's value\n", o.HeaderEnv)
	}
	if o.ServiceName != "" {
		fmt.Fprintf(b, "service_name = %q   # service.name on every span; default reasonix\n", o.ServiceName)
	}
	b.WriteString("\n")
}

var (
	ErrOTLPEndpointInvalid     = errors.New("otlp endpoint must be an http or https URL with a host")
	ErrOTLPEndpointCredentials = errors.New("otlp endpoint must not carry credentials, a query or a fragment")
	ErrOTLPHeaderInvalid       = errors.New("otlp header name is not a plain HTTP header name")
	ErrOTLPHeaderEnvInvalid    = errors.New("otlp header_env must be an OTEL_* or REASONIX_OTEL_* variable")
)

var otlpHeaderName = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

var otlpReservedHeaders = map[string]bool{"host": true, "content-length": true, "content-type": true, "transfer-encoding": true}

// ParseOTLPEndpoint is the one place an export endpoint is judged: an http(s)
// URL with a host and nothing in it that could carry a secret.
func ParseOTLPEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, ErrOTLPEndpointInvalid
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, ErrOTLPEndpointCredentials
	}
	return u, nil
}

// OTLPHeaderNameOK reports whether name may carry the export credential.
func OTLPHeaderNameOK(name string) bool {
	return otlpHeaderName.MatchString(name) && !otlpReservedHeaders[strings.ToLower(name)]
}

// SetOTLPEndpoint sets the collector address; empty clears it.
func (c *Config) SetOTLPEndpoint(v string) error {
	v = strings.TrimSpace(v)
	if v != "" {
		if _, err := ParseOTLPEndpoint(v); err != nil {
			return err
		}
	}
	c.Observability.OTLP.Endpoint = v
	return nil
}

// SetOTLPHeaderName sets the header that carries the credential; empty clears it.
func (c *Config) SetOTLPHeaderName(v string) error {
	if v != "" && !OTLPHeaderNameOK(v) {
		return ErrOTLPHeaderInvalid
	}
	c.Observability.OTLP.HeaderName = v
	return nil
}

// SetOTLPHeaderEnv sets which variable holds the credential's value.
func (c *Config) SetOTLPHeaderEnv(v string) error {
	if !OTLPHeaderEnvOK(v) {
		return ErrOTLPHeaderEnvInvalid
	}
	c.Observability.OTLP.HeaderEnv = v
	return nil
}
