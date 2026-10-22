package config

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestProjectConfigFallback(t *testing.T) {
	for _, mode := range []string{"missing", "nested", "both", "invalid-root", "root-directory"} {
		t.Run(mode, func(t *testing.T) {
			isolateUserConfigHome(t)
			root := testenv.TempDir(t)
			primary := filepath.Join(root, "reasonix.toml")
			nested := filepath.Join(root, ".reasonix", "reasonix.toml")
			want := primary
			if mode != "missing" {
				if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(nested, []byte("[agent]\ncompact_ratio = 0.61\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "nested":
				want = nested
			case "both":
				if err := os.WriteFile(primary, []byte("[agent]\ncompact_ratio = 0.72\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "invalid-root":
				if err := os.WriteFile(primary, []byte("[broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "root-directory":
				if err := os.Mkdir(primary, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got := ProjectConfigPath(root); got != want {
				t.Fatalf("path = %q, want %q", got, want)
			}
			cfg, err := LoadForRootReadOnly(root)
			if mode == "invalid-root" || mode == "root-directory" {
				if err == nil && cfg.Agent.CompactRatio == 0.61 {
					t.Fatal("invalid root config fell through to nested config")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing" {
				return
			}
			ratio := 0.61
			if mode == "both" {
				ratio = 0.72
			}
			if cfg.Agent.CompactRatio != ratio {
				t.Fatalf("compact_ratio = %v, want %v", cfg.Agent.CompactRatio, ratio)
			}
			if got := SourcePathForRoot(root); got != want {
				t.Fatalf("source = %q, want %q", got, want)
			}
			entry := PluginEntry{Source: MCPSourceProjectConfig}
			if got := MCPConfigPathForEntry(root, entry); got != want {
				t.Fatalf("MCP path = %q, want %q", got, want)
			}
			if _, err := SaveMinimalProjectCompactRatio(ProjectConfigPath(root), 0.65); err != nil {
				t.Fatal(err)
			}
			reloaded, err := LoadForRootReadOnly(root)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Agent.CompactRatio != 0.65 {
				t.Fatalf("saved ratio = %v", reloaded.Agent.CompactRatio)
			}
			if mode == "nested" {
				if _, err := os.Lstat(primary); !os.IsNotExist(err) {
					t.Fatalf("save created root config: %v", err)
				}
			}
		})
	}
}

func TestProjectConfigFallbackRelativeRootAndScope(t *testing.T) {
	isolateUserConfigHome(t)
	root := testenv.TempDir(t)
	t.Chdir(root)
	if err := os.Mkdir(".reasonix", 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(".reasonix", "reasonix.toml")
	body := "[agent]\ncompact_ratio = 0.61\n[desktop]\ndefault_tool_approval_mode = \"yolo\"\n"
	if err := os.WriteFile(nested, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"", "."} {
		if got := ProjectConfigPath(root); got != nested {
			t.Fatalf("path for %q = %q", root, got)
		}
	}
	cfg, err := LoadForRootReadOnly(".")
	if err != nil {
		t.Fatal(err)
	}
	if NormalizeToolApprovalMode(cfg.Desktop.DefaultToolApprovalMode) == "yolo" {
		t.Fatal("nested config changed user-only approval mode")
	}
	if err := cfg.SaveForRoot("."); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("reasonix.toml"); !os.IsNotExist(err) {
		t.Fatalf("SaveForRoot created root config: %v", err)
	}
	paths := mcpConfigSourcePathsForRoot(".")
	for _, want := range ProjectConfigPaths(".") {
		found := false
		for _, path := range paths {
			found = found || path == want
		}
		if !found {
			t.Fatalf("MCP transaction does not lock %q: %v", want, paths)
		}
	}
	if err := os.WriteFile("reasonix.toml", []byte("[agent]\ncompact_ratio = 0.72\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadForRootReadOnly(".")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.CompactRatio != 0.72 {
		t.Fatal("reload did not switch to root config")
	}
	if err := os.Remove("reasonix.toml"); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadForRootReadOnly(".")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.CompactRatio != 0.61 {
		t.Fatal("reload did not return to nested config")
	}
}

func TestProjectConfigFallbackRejectsEscapingDirectory(t *testing.T) {
	isolateUserConfigHome(t)
	root, outside := testenv.TempDir(t), testenv.TempDir(t)
	if err := os.WriteFile(filepath.Join(outside, "reasonix.toml"), []byte("[agent]\ncompact_ratio = 0.61\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".reasonix")); err != nil {
		t.Skip(err)
	}
	if _, err := LoadForRootReadOnly(root); err == nil {
		t.Fatal("loaded project config outside project")
	}
	if _, err := SaveMinimalProjectCompactRatio(ProjectConfigPath(root), 0.65); err == nil {
		t.Fatal("wrote project config outside project")
	}
}

func TestProjectConfigFallbackRejectsEscapingEmptyDirectory(t *testing.T) {
	isolateUserConfigHome(t)
	root, outside := testenv.TempDir(t), testenv.TempDir(t)
	if err := os.Symlink(outside, filepath.Join(root, ".reasonix")); err != nil {
		t.Skip(err)
	}
	if _, err := LoadForRootReadOnly(root); err == nil {
		t.Fatal("escaping directory was treated as an absent nested config")
	}
}

func TestProjectConfigFallbackAllowsInternalDirectoryLink(t *testing.T) {
	isolateUserConfigHome(t)
	root := testenv.TempDir(t)
	dir := filepath.Join(root, "local-config")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reasonix.toml"), []byte("[agent]\ncompact_ratio = 0.61\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, filepath.Join(root, ".reasonix")); err != nil {
		t.Skip(err)
	}
	cfg, err := LoadForRootReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.CompactRatio != 0.61 {
		t.Fatalf("internal directory link was not loaded: %v", cfg.Agent.CompactRatio)
	}
}

func TestProjectConfigFallbackKeepsBrokenRootLink(t *testing.T) {
	root := testenv.TempDir(t)
	primary := filepath.Join(root, "reasonix.toml")
	nested := filepath.Join(root, ".reasonix", "reasonix.toml")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing.toml"), primary); err != nil {
		t.Skip(err)
	}
	if got := ProjectConfigPath(root); got != primary {
		t.Fatalf("broken root link fell through: %q", got)
	}
}
