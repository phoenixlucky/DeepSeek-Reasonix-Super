package config

import (
	"fmt"
	"strings"
)

// renderBashAndNetwork writes the sandbox's mode and network lines of the user
// file, and windows_integrity beside them when it is on: without it the mode
// written for Windows would read back as the default.
func renderBashAndNetwork(b *strings.Builder, c *Config) {
	fmt.Fprintf(b, "bash    = %q\n", c.BashMode())
	if c.Sandbox.WindowsIntegrity {
		b.WriteString("windows_integrity = true   # Windows only: bash runs under a write-restricted token; user file only\n")
	}
	fmt.Fprintf(b, "network = %v\n", c.Sandbox.Network)
}
