package config

import (
	"slices"
	"strings"
	"testing"
)

func TestUserFileChoosesMCPResources(t *testing.T) {
	if Default().Tools.MCPResources {
		t.Fatal("reading MCP resources is on by default")
	}
	cfg, _ := loadScoped(t, "[tools]\nmcp_resources = true\n", "")
	if !cfg.Tools.MCPResources {
		t.Fatal("a user file's mcp_resources was not honoured")
	}
}

func TestProjectCannotTurnOnMCPResources(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[tools]\nmcp_resources = true\n")
	if cfg.Tools.MCPResources {
		t.Fatal("a cloned repo's reasonix.toml let the model read a server's resources")
	}
	if !slices.Contains(ignoredKeys(cfg), "tools.mcp_resources") {
		t.Fatalf("ignored = %v, want tools.mcp_resources reported", ignoredKeys(cfg))
	}
	kept, _ := loadScoped(t, "[tools]\nmcp_resources = true\n", "[tools]\nmcp_resources = false\n")
	if !kept.Tools.MCPResources {
		t.Fatal("the project file overrode the user's own choice")
	}
}

func TestMCPResourcesSurvivesASave(t *testing.T) {
	cfg := Default()
	cfg.Tools.MCPResources = true
	if out := RenderTOMLForScope(cfg, RenderScopeUser); !strings.Contains(out, "mcp_resources = true") {
		t.Fatalf("rendered config lost the choice:\n%s", out)
	}
}
