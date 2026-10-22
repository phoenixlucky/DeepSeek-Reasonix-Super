package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The host runs a scheduled child through the small schedulerun package. The
// command line is not part of what it ships, and importing it again for one
// entry point would bring back every command it carries.
func TestTheHostDoesNotCarryTheCommandLine(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list is not available: %v", err)
	}
	for pkg := range strings.FieldsSeq(string(out)) {
		if pkg == "reasonix/internal/frontend/cli" {
			t.Fatal("the studio host depends on the command line package")
		}
	}
}
