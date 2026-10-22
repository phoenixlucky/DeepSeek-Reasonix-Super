package config

import (
	"slices"
	"strings"
	"testing"
)

// On Windows the bash sandbox is the user's explicit choice: an older config's
// "enforce" does not switch it on, and only windows_integrity does.
func TestWindowsEnforcesOnlyWhenWindowsIntegrityIsOn(t *testing.T) {
	cfg := Default()
	if got := cfg.BashModeForGOOS("windows"); got != "off" {
		t.Fatalf("default on Windows = %q, want off", got)
	}
	cfg.Sandbox.Bash = "enforce"
	if got := cfg.BashModeForGOOS("windows"); got != "off" {
		t.Fatalf(`an explicit bash = "enforce" turned the Windows sandbox on: %q`, got)
	}
	cfg.Sandbox.WindowsIntegrity = true
	if got := cfg.BashModeForGOOS("windows"); got != "enforce" {
		t.Fatalf("windows_integrity = true gave %q, want enforce", got)
	}
	if got := cfg.BashModeForGOOS("linux"); got != "enforce" {
		t.Fatalf("Linux is unaffected: %q", got)
	}
}

// Turning it on edits ACLs on the user's disk, so a cloned repository cannot.
func TestAProjectFileCannotTurnTheWindowsSandboxOnOrOff(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[sandbox]\nwindows_integrity = true\n")
	if cfg.Sandbox.WindowsIntegrity {
		t.Fatal("a cloned repo's reasonix.toml switched the Windows sandbox on")
	}
	if !slices.Contains(ignoredKeys(cfg), "sandbox.windows_integrity") {
		t.Fatalf("ignored = %v, want sandbox.windows_integrity reported", ignoredKeys(cfg))
	}
	cfg, _ = loadScoped(t, "[sandbox]\nwindows_integrity = true\n", "[sandbox]\nwindows_integrity = false\n")
	if !cfg.Sandbox.WindowsIntegrity {
		t.Fatal("the project file switched off what the user turned on")
	}
}

// A save must keep the switch: the mode alone reads back as the Windows default.
func TestWindowsIntegritySurvivesASaveAndReload(t *testing.T) {
	cfg := Default()
	cfg.Sandbox.WindowsIntegrity = true
	text := RenderTOMLForScope(cfg, RenderScopeUser)
	if !strings.Contains(text, "windows_integrity = true") {
		t.Fatalf("the rendered user file lost the switch:\n%s", text)
	}
	off := Default()
	if strings.Contains(RenderTOMLForScope(off, RenderScopeUser), "windows_integrity") {
		t.Fatal("an unset switch was written out")
	}
}

// A caller's request to enforce (an observed run asks for it) does not make
// Windows edit permissions on the user's folders unless the user turned the
// sandbox on; elsewhere the request is honoured.
func TestAnEnforceOverrideNeedsTheUsersConsentOnWindows(t *testing.T) {
	old := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = old })
	cfg := Default()

	runtimeGOOS = "windows"
	if got := cfg.BashModeUnderOverride("enforce"); got != "off" {
		t.Fatalf("windows, no opt-in: %q, want off", got)
	}
	if got := cfg.BashModeUnderOverride("off"); got != "off" {
		t.Fatalf("windows, off override: %q", got)
	}
	cfg.Sandbox.WindowsIntegrity = true
	if got := cfg.BashModeUnderOverride("enforce"); got != "enforce" {
		t.Fatalf("windows, opted in: %q, want enforce", got)
	}
	runtimeGOOS = "linux"
	cfg.Sandbox.WindowsIntegrity = false
	if got := cfg.BashModeUnderOverride("enforce"); got != "enforce" {
		t.Fatalf("linux: %q, want the override", got)
	}
}
