package boot

import (
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/safety/sandbox"
)

// effectiveBashMode is the bash-sandbox mode this build runs under.
func effectiveBashMode(opts Options, cfg *config.Config) string {
	if override := strings.TrimSpace(opts.SandboxBashOverride); override != "" {
		return cfg.BashModeUnderOverride(override)
	}
	return cfg.BashMode()
}

// shellConfinedToFolder reports whether an OS sandbox in force keeps a shell
// command's writes inside the session's write roots. Only then may the lease
// claim just the session's folder for a writer that declared nothing.
func shellConfinedToFolder(opts Options, cfg *config.Config) bool {
	network := cfg.Sandbox.Network
	if opts.SandboxNetworkOverride != nil {
		network = *opts.SandboxNetworkOverride
	}
	spec := sandbox.Spec{Mode: effectiveBashMode(opts, cfg), Network: network}
	return spec.Enforce() && sandbox.AvailableFor(spec)
}
