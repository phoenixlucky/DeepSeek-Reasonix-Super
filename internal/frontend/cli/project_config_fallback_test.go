package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
)

func TestLocalConfigCommandsUseNestedProjectConfig(t *testing.T) {
	isolateCLIConfigHome(t)
	if err := os.Mkdir(".reasonix", 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(".reasonix", "reasonix.toml")
	if err := os.WriteFile(nested, []byte("[agent]\ncompact_ratio = 0.61\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolveSetupTargets([]string{"--local"}).config; got != nested {
		t.Fatalf("setup target = %q", got)
	}
	for _, args := range [][]string{{"config", "compact-ratio", "--local", "70"}, {"config", "reasoning-language", "--local", "en"}} {
		captureStdout(t, func() {
			if rc := Run(args, "test"); rc != 0 {
				t.Fatalf("%v: exit %d", args, rc)
			}
		})
	}
	cfg, err := config.LoadForRootReadOnly(".")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.CompactRatio != 0.7 || cfg.ReasoningLanguage() != "en" {
		t.Fatalf("nested config edits not loaded: %+v", cfg.Agent)
	}
	if !strings.Contains(compactRatioSource(), nested) {
		t.Fatalf("wrong source: %q", compactRatioSource())
	}
	if _, err := os.Stat("reasonix.toml"); !os.IsNotExist(err) {
		t.Fatalf("created root config: %v", err)
	}
}
