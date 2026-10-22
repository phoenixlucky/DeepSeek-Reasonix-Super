package repair

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestRepairUsesNestedProjectConfig(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	path := filepath.Join(root, ".reasonix", "reasonix.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := projectConfigPath(root); got != path {
		t.Fatalf("plan path = %q, want %q", got, path)
	}
	report, err := InspectAndRepairConfig(ConfigOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if report.Checks[1].Path != path || report.Checks[1].Valid {
		t.Fatalf("nested check = %+v", report.Checks[1])
	}
	report, err = InspectAndRepairConfig(ConfigOptions{Root: root, Apply: true, IncludeProject: true, OnlyScope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Checks[1].Exists || !report.Checks[1].Valid {
		t.Fatalf("after repair = %+v", report.Checks[1])
	}
	if matches, _ := filepath.Glob(path + ".reasonix-quarantine-*"); len(matches) != 1 {
		t.Fatalf("quarantine = %v", matches)
	}
}
