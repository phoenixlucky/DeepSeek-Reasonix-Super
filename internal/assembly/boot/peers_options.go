package boot

import (
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/session/control"
)

// peersOptions gives a session the spool directory the sandbox already denies to
// confined commands, but opens it only when the user file asked for messages
// between sessions; otherwise the session waits on standby for the switch in
// settings. The link lets the peer tools reach the controller once it exists.
func peersOptions(cfg *config.Config, link *tool.PeerLink, tools func(enabled, dispatch bool)) control.PeersOptions {
	return control.PeersOptions{
		Dir: cfg.Roots().PeersDir(), Link: link, Tools: tools,
		Standby: !cfg.PeersEnabled(), Dispatch: cfg.PeersDispatch(),
	}
}
