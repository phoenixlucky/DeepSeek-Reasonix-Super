package cli

import (
	"os"

	"reasonix/internal/base/secrets"
)

// exitAgentChild is the status of a command refused because an agent tool
// started it. It is the same status the person-only schedule commands use.
const exitAgentChild = 77

// inAgentChild reports whether the host started this process for an agent tool.
// The marker is inherited by every subprocess the host launches for the agent; a
// child can clear its own environment, so this guards against an agent that does
// not try to hide, and it is not the sandbox's boundary.
func inAgentChild() bool { return os.Getenv(secrets.AgentChildEnv) == "1" }
