package installsource

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestInstallSourceUsesNestedProjectConfig(t *testing.T) {
	root := testenv.TempDir(t)
	path := filepath.Join(root, ".reasonix", "reasonix.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := &Tool{root: root}
	if got := tool.configPath("project"); got != path {
		t.Fatalf("install target = %q, want %q", got, path)
	}
}
