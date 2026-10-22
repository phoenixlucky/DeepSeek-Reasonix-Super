package mcpsetup

import (
	"strings"
	"testing"
)

func TestParseArgsReturnsReadableUsageErrorsWithoutEchoingFlagValues(t *testing.T) {
	_, err := ParseArgs([]string{"docs", "--unsupported=secret-value"})
	if err == nil {
		t.Fatal("ParseArgs succeeded, want an error")
	}
	if !strings.Contains(err.Error(), `unknown flag "--unsupported"`) {
		t.Fatalf("error = %q, want the unknown flag name", err)
	}
	if strings.Contains(err.Error(), "dependency failure") || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("error leaked dependency wording or flag value: %q", err)
	}

	_, err = ParseArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "missing server name, command, or URL") {
		t.Fatalf("ParseArgs(nil) error = %v, want readable missing-argument guidance", err)
	}

	_, err = ParseArgs([]string{"--api-key=secret-value"})
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("invalid first argument error leaked its value: %v", err)
	}
}
