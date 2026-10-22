package config

import (
	"slices"
	"strings"
	"testing"
)

func TestVideoAddressesAreHandedToTheVendorOnlyWhenTheUserFileSaysSo(t *testing.T) {
	if Default().Tools.VideoURLPassthrough {
		t.Fatal("address pass-through is on by default")
	}
	cfg, _ := loadScoped(t, "[tools]\nvideo_url_passthrough = true\n", "")
	if !cfg.Tools.VideoURLPassthrough {
		t.Fatal("the user file's switch was not honoured")
	}
	if out := RenderTOMLForScope(cfg, RenderScopeUser); !strings.Contains(out, "video_url_passthrough = true") {
		t.Fatalf("the switch did not survive a save:\n%s", out)
	}
}

func TestAProjectFileCannotTurnAddressPassThroughOn(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[tools]\nvideo_url_passthrough = true\n")
	if cfg.Tools.VideoURLPassthrough || !slices.Contains(ignoredKeys(cfg), "tools.video_url_passthrough") {
		t.Fatalf("a cloned repo turned the switch on: %+v ignored=%v", cfg.Tools, ignoredKeys(cfg))
	}
}

func TestAProjectFileCannotTurnAddressPassThroughOff(t *testing.T) {
	cfg, _ := loadScoped(t, "[tools]\nvideo_url_passthrough = true\n", "[tools]\nvideo_url_passthrough = false\n")
	if !cfg.Tools.VideoURLPassthrough {
		t.Fatal("the project file overrode the user's own choice")
	}
}
