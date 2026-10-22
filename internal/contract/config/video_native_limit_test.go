package config

import (
	"slices"
	"strings"
	"testing"
)

func TestUserFileLowersTheNativeVideoSizeLimit(t *testing.T) {
	if Default().Tools.VideoNativeMaxBytes != 0 {
		t.Fatal("a native video size limit is set by default")
	}
	cfg, _ := loadScoped(t, "[tools]\nvideo_native_max_bytes = 5000000\n", "")
	if cfg.Tools.VideoNativeMaxBytes != 5_000_000 {
		t.Fatalf("user file's limit was not honoured: %+v", cfg.Tools)
	}
	out := RenderTOMLForScope(cfg, RenderScopeUser)
	if !strings.Contains(out, "video_native_max_bytes = 5000000") {
		t.Fatalf("the limit did not survive a save:\n%s", out)
	}
}

func TestProjectCannotSetTheNativeVideoSizeLimit(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[tools]\nvideo_native_max_bytes = 1\n")
	if cfg.Tools.VideoNativeMaxBytes != 0 || !slices.Contains(ignoredKeys(cfg), "tools.video_native_max_bytes") {
		t.Fatalf("a cloned repo set the user's video limit: %+v ignored=%v", cfg.Tools, ignoredKeys(cfg))
	}
	kept, _ := loadScoped(t, "[tools]\nvideo_native_max_bytes = 5000000\n", "[tools]\nvideo_native_max_bytes = 1\n")
	if kept.Tools.VideoNativeMaxBytes != 5_000_000 {
		t.Fatal("the project file overrode the user's own limit")
	}
}
