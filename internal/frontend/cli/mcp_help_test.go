package cli

import (
	"strings"
	"testing"
)

func TestMCPAddHelpPrintsUsage(t *testing.T) {
	var code int
	stdout, stderr := captureCLIOutput(t, func() {
		code = Run([]string{"mcp", "add", "--help"}, "test")
	})
	if code != 0 {
		t.Fatalf("mcp add --help exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "reasonix mcp add") || stderr != "" {
		t.Fatalf("mcp add --help output omitted usage or wrote to stderr\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}
