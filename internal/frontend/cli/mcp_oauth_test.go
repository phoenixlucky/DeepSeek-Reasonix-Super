package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/plugin"
)

func TestMCPAuthCLIUsesReasonixPrivateState(t *testing.T) {
	home, workspace := testenv.TempDir(t), testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Chdir(workspace)
	entry := config.PluginEntry{Name: "figma", Type: "http", URL: "https://mcp.figma.com/mcp", Source: config.MCPSourceUserConfig}
	if _, err := config.InstallUserPluginForRoot(workspace, entry, true); err != nil {
		t.Fatal(err)
	}

	previous := mcpAuthorizeForCLI
	mcpAuthorizeForCLI = func(_ context.Context, spec plugin.Spec, openURL func(string) error) error {
		if spec.Name != "figma" || spec.URL != entry.URL {
			t.Fatalf("authorization spec = %+v", spec)
		}
		if spec.StateDir == "" || strings.HasPrefix(filepath.Clean(spec.StateDir), filepath.Clean(workspace)+string(filepath.Separator)) {
			t.Fatalf("OAuth state dir must be private Reasonix state, got %q", spec.StateDir)
		}
		if spec.OAuthHTTPClient == nil || openURL == nil {
			t.Fatal("authorization requires the proxy-aware client and browser opener")
		}
		return nil
	}
	t.Cleanup(func() { mcpAuthorizeForCLI = previous })
	if code := mcpAuthCLI([]string{"figma"}); code != 0 {
		t.Fatalf("mcp auth exit = %d", code)
	}
}

func TestMCPAuthCLIRejectsIneligibleConfigurations(t *testing.T) {
	tests := []struct {
		name  string
		entry config.PluginEntry
	}{
		{name: "stdio", entry: config.PluginEntry{Name: "server", Type: "stdio", Command: "server-mcp"}},
		{name: "legacy SSE", entry: config.PluginEntry{Name: "server", Type: "sse", URL: "https://mcp.example.test/sse"}},
		{name: "static authentication", entry: config.PluginEntry{
			Name: "server", Type: "http", URL: "https://mcp.example.test/mcp",
			Headers: map[string]string{"Authorization": "Bearer configured"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateCLIConfigHome(t)
			workspace := testenv.TempDir(t)
			t.Chdir(workspace)
			tc.entry.Source = config.MCPSourceUserConfig
			if _, err := config.InstallUserPluginForRoot(workspace, tc.entry, true); err != nil {
				t.Fatal(err)
			}

			called := false
			previous := mcpAuthorizeForCLI
			mcpAuthorizeForCLI = func(context.Context, plugin.Spec, func(string) error) error {
				called = true
				return nil
			}
			t.Cleanup(func() { mcpAuthorizeForCLI = previous })
			if code := mcpAuthCLI([]string{"server"}); code == 0 {
				t.Fatal("mcp auth unexpectedly accepted an ineligible server")
			}
			if called {
				t.Fatal("ineligible server reached the OAuth implementation")
			}
		})
	}
}

// The terminal names the host before the browser opens, so the person typing the
// command sees where the server sends them.
func TestMCPAuthCLINamesTheHostBeforeOpeningTheBrowser(t *testing.T) {
	isolateCLIConfigHome(t)
	workspace := testenv.TempDir(t)
	t.Chdir(workspace)
	entry := config.PluginEntry{Name: "figma", Type: "http", URL: "https://mcp.figma.com/mcp", Source: config.MCPSourceUserConfig}
	if _, err := config.InstallUserPluginForRoot(workspace, entry, true); err != nil {
		t.Fatal(err)
	}
	var opened string
	prevAuth, prevOpen := mcpAuthorizeForCLI, mcpOpenForCLI
	mcpOpenForCLI = func(raw string) error { opened = raw; return nil }
	mcpAuthorizeForCLI = func(_ context.Context, _ plugin.Spec, open func(string) error) error {
		return open("https://auth.example.test:8443/authorize?x=1")
	}
	t.Cleanup(func() { mcpAuthorizeForCLI, mcpOpenForCLI = prevAuth, prevOpen })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	code := mcpAuthCLI([]string{"figma"})
	os.Stdout = stdout
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if code != 0 || opened == "" {
		t.Fatalf("exit=%d opened=%q", code, opened)
	}
	if !strings.Contains(string(out), "auth.example.test:8443") {
		t.Fatalf("the host was not named:\n%s", out)
	}
}

func TestMCPAuthCLIUpgradeNeedsAServerThatAsked(t *testing.T) {
	home, workspace := testenv.TempDir(t), testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Chdir(workspace)
	entry := config.PluginEntry{Name: "figma", Type: "http", URL: "https://mcp.figma.com/mcp", Source: config.MCPSourceUserConfig}
	if _, err := config.InstallUserPluginForRoot(workspace, entry, true); err != nil {
		t.Fatal(err)
	}
	previous := mcpAuthorizeForCLI
	called := false
	mcpAuthorizeForCLI = func(_ context.Context, spec plugin.Spec, _ func(string) error) error {
		called = true
		if !spec.OAuthScopeUpgrade {
			t.Fatal("the upgrade flag did not reach the authorization")
		}
		return nil
	}
	t.Cleanup(func() { mcpAuthorizeForCLI = previous })
	if code := mcpAuthCLI([]string{"figma", "--upgrade"}); code != 1 || called {
		t.Fatalf("nothing asked: exit=%d called=%v", code, called)
	}
	if code := mcpAuthCLI([]string{"figma", "--bogus"}); code != 2 {
		t.Fatalf("unknown flag exit = %d", code)
	}
}
