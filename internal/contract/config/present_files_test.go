package config

import (
	"slices"
	"strings"
	"testing"
)

func TestPresentFilesIsUserOptIn(t *testing.T) {
	if Default().Tools.PresentFiles {
		t.Fatal("present_files is on by default")
	}
	for _, enabled := range []bool{false, true} {
		user, project := "", "[tools]\npresent_files = true\n"
		if enabled {
			user, project = project, "[tools]\npresent_files = false\n"
		}
		cfg, _ := loadScoped(t, user, project)
		if cfg.Tools.PresentFiles != enabled {
			t.Fatal("project changed the user's delivery choice")
		}
		if !slices.Contains(ignoredKeys(cfg), "tools.present_files") {
			t.Fatal("ignored project key not reported")
		}
		out := RenderTOMLForScope(cfg, RenderScopeUser)
		if strings.Contains(out, "present_files = true") != enabled {
			t.Fatalf("save changed choice: %s", out)
		}
		loaded, _ := loadScoped(t, out, "")
		if loaded.Tools.PresentFiles != enabled {
			t.Fatal("saved choice did not round-trip")
		}
	}
}
