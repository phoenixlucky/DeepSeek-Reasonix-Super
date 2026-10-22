package boot

import (
	"context"
	"fmt"
	"io"
	"time"

	"reasonix/internal/base/netclient"
	"reasonix/internal/contract/config"
	"reasonix/internal/platform/otlp"
)

const traceCloseBudget = 3 * time.Second

// newTraceExporter starts the opt-in OTLP export when the user's own config
// switches it on and names a collector; otherwise nothing is built and nothing
// leaves the machine. A setting that cannot be honoured is reported once and
// leaves the export off.
func newTraceExporter(cfg *config.Config, root, version string, stderr io.Writer) *otlp.Exporter {
	if !cfg.OTLPActive() {
		return nil
	}
	o := cfg.Observability.OTLP
	secret := ""
	if o.HeaderEnv != "" && config.OTLPHeaderEnvOK(o.HeaderEnv) {
		secret = config.ResolveCredentialForRoot(root, o.HeaderEnv).Value
	}
	if o.HeaderName != "" && secret == "" {
		fmt.Fprintf(stderr, "trace export: %s is empty; sending without a credential\n", o.HeaderEnv)
	}
	client, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{
		DialTimeout: 3 * time.Second, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(stderr, "trace export is off: %v\n", err)
		return nil
	}
	exp, err := otlp.New(otlp.Settings{Endpoint: o.Endpoint, HeaderName: o.HeaderName, HeaderValue: secret, ServiceName: o.ServiceName, Version: version}, client)
	if err != nil {
		fmt.Fprintf(stderr, "trace export is off: %v\n", err)
		return nil
	}
	return exp
}

// closeAfter stops the exporter, flushing what is queued, once cleanup ends.
func closeAfter(cleanup func(), exp *otlp.Exporter) func() {
	if exp == nil {
		return cleanup
	}
	return func() {
		cleanup()
		ctx, cancel := context.WithTimeout(context.Background(), traceCloseBudget)
		defer cancel()
		_ = exp.Close(ctx)
	}
}
