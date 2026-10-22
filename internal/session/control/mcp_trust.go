package control

import (
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/ext/plugin"
)

// ErrMCPNothingHeld is a trust request for a server with no held definitions.
var ErrMCPNothingHeld = plugin.ErrNoHeldTools

// ErrMCPDigestMismatch is a trust request bound to definitions the server no
// longer holds.
var ErrMCPDigestMismatch = plugin.ErrHeldDigestMismatch

// AcceptMCPHeldTools approves the definitions the server's live connection
// withheld, if digest names exactly that set, and reconnects it so those tools register. It returns the tool count
// of the reconnected server.
func (c *Controller) AcceptMCPHeldTools(name, digest string) (int, error) {
	name = strings.TrimSpace(name)
	host := c.mcp.hostRef()
	if host == nil {
		return 0, ErrMCPNothingHeld
	}
	if _, err := host.AcceptHeldTools(name, strings.TrimSpace(digest)); err != nil {
		return 0, fmt.Errorf("accept held tools of %q: %w", name, err)
	}
	return c.ReconnectMCPServer(name)
}

// ErrMCPFollowUnwritable is the approval record that could not be read or written.
var ErrMCPFollowUnwritable = errors.New("could not record the choice")

// SetMCPFollowToolChanges records whether the user takes this server's tool
// definition changes without being asked, then reconnects it so what is held
// now follows the choice. Only a server the user authorized can be followed: a
// project server waiting for approval stays held whatever is recorded here.
func (c *Controller) SetMCPFollowToolChanges(name string, follow bool) (int, error) {
	name = strings.TrimSpace(name)
	entry, err := c.configuredMCPServer(name)
	if err != nil {
		return 0, err
	}
	if err := plugin.SetToolPinsFollow(c.mcpSpec(entry), follow); err != nil {
		return 0, fmt.Errorf("%w for %q: %w", ErrMCPFollowUnwritable, name, err)
	}
	if host := c.mcp.hostRef(); host == nil || !follow {
		return 0, nil
	} else if held, _ := host.HeldTools(name); len(held) == 0 {
		return 0, nil
	}
	return c.ReconnectMCPServer(name)
}
