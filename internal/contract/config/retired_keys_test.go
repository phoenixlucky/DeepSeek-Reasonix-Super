package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	fileencoding "reasonix/internal/base/fileutil/encoding"
	"reasonix/internal/base/testenv"
)

const allRetiredKeys = `default_model = "deepseek-pro"

[agent]
max_steps = 9
memory_compiler = "compact"
soft_compact_ratio = 0.5
temperature = 0.7

[secrets]
redact_tool_output = true
filter_subprocess_env = true
`

func realPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

func countingReader(mu *sync.Mutex, reads map[string]int) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		mu.Lock()
		reads[realPath(path)]++
		mu.Unlock()
		return fileencoding.ReadFileUTF8(path)
	}
}

func writeRetiredFixtures(t *testing.T) (root, userPath, projectPath string) {
	t.Helper()
	isolateUserConfigHome(t)
	root = testenv.TempDir(t)
	userPath = UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath = filepath.Join(root, "reasonix.toml")
	for _, p := range []string{userPath, projectPath} {
		if err := os.WriteFile(p, []byte(allRetiredKeys), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, userPath, projectPath
}

// Four independent migrations over the same user and project files see them
// once to learn there is work and once under the lock to do it, and the result
// is the same text four sequential rewrites left.
func TestRetiredKeyMigrationsReadEachConfigOncePerPhase(t *testing.T) {
	root, userPath, projectPath := writeRetiredFixtures(t)
	var mu sync.Mutex
	reads := map[string]int{}
	all := []retiredKey{retiredStepLimits, retiredRedactToolOutput, retiredMemoryCompiler, retiredMultiThreshold}

	res := processRoots().migrateRetiredKeys(root, countingReader(&mu, reads), nil, all)
	for i, r := range res {
		if r.Err != nil || !r.Changed {
			t.Fatalf("migration %d = %+v, want changed without error", i, r)
		}
	}
	for _, p := range []string{userPath, projectPath} {
		if reads[realPath(p)] != 2 {
			t.Fatalf("%s read %d times, want twice: %v", p, reads[realPath(p)], reads)
		}
		raw, _ := os.ReadFile(p)
		want := "default_model = \"deepseek-pro\"\n\n[agent]\ntemperature = 0.7\n\n[secrets]\nfilter_subprocess_env = true\n"
		if string(raw) != want {
			t.Fatalf("%s after migration:\n%s\nwant:\n%s", p, raw, want)
		}
	}

	again := processRoots().migrateRetiredKeys(root, countingReader(&mu, reads), nil, all)
	for i, r := range again {
		if r.Changed || r.Err != nil {
			t.Fatalf("second run migration %d = %+v, want one-shot", i, r)
		}
	}
}

// The counter sees what the separate migrations cost: two reads of each file per
// migration.
func TestRetiredKeyMigrationsRunSeparatelyReadPerMigration(t *testing.T) {
	root, userPath, _ := writeRetiredFixtures(t)
	var mu sync.Mutex
	reads := map[string]int{}
	for _, m := range []retiredKey{retiredStepLimits, retiredRedactToolOutput, retiredMemoryCompiler, retiredMultiThreshold} {
		processRoots().migrateRetiredKeys(root, countingReader(&mu, reads), nil, []retiredKey{m})
	}
	if got := reads[realPath(userPath)]; got != 8 {
		t.Fatalf("separate migrations read the user config %d times, want 8", got)
	}
}

// A migration that finds nothing to strip leaves the others' result and the
// file untouched by it.
func TestRetiredKeyMigrationsReportChangeIndependently(t *testing.T) {
	isolateUserConfigHome(t)
	root := testenv.TempDir(t)
	userPath := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte("[agent]\nmemory_compiler = \"compact\"\ntemperature = 0.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := processRoots().MigrateRetiredKeysForRoot(root)
	if !got.MemoryCompiler.Changed || got.StepLimits.Changed || got.RedactToolOutput.Changed || got.MultiThreshold.Changed {
		t.Fatalf("results = %+v, want only memory_compiler changed", got)
	}
	raw, _ := os.ReadFile(userPath)
	if strings.Contains(string(raw), "memory_compiler") || !strings.Contains(string(raw), "temperature = 0.7") {
		t.Fatalf("user config after migration:\n%s", raw)
	}
}

// A failing file stops every migration at that file with its own wrapped error.
func TestRetiredKeyMigrationsErrorsNameEachMigration(t *testing.T) {
	isolateUserConfigHome(t)
	root := testenv.TempDir(t)
	userPath := UserConfigPath()
	if err := os.MkdirAll(userPath, 0o755); err != nil {
		t.Fatal(err)
	}
	got := processRoots().MigrateRetiredKeysForRoot(root)
	for name, r := range map[string]RetiredKeyResult{
		"deprecated agent step limits":               got.StepLimits,
		"deprecated redact_tool_output":              got.RedactToolOutput,
		"deprecated memory_compiler":                 got.MemoryCompiler,
		"deprecated multi-threshold compaction keys": got.MultiThreshold,
	} {
		if r.Err == nil || !strings.Contains(r.Err.Error(), "migrate "+name+" in "+userPath) || r.Changed {
			t.Fatalf("%s result = %+v, want its own error naming the file", name, r)
		}
	}
}

// A config with no retired key is read once and never takes the edit lock: the
// startup migrations of every Build pass through here, and the lock is a file
// operation plus path resolution that has nothing to protect.
func TestRetiredKeyMigrationsLeaveACleanConfigUnlocked(t *testing.T) {
	isolateUserConfigHome(t)
	root := testenv.TempDir(t)
	userPath := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte("[agent]\ntemperature = 0.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, err := LockConfigFileEdits(userPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	done := make(chan RetiredKeyMigrations, 1)
	go func() { done <- processRoots().MigrateRetiredKeysForRoot(root) }()
	select {
	case got := <-done:
		if got != (RetiredKeyMigrations{}) {
			t.Fatalf("results = %+v, want nothing to do", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a config with no retired key waited for the edit lock")
	}
}

func TestDeepSeekProtocolMigrationLeavesAnUpToDateConfigUnlocked(t *testing.T) {
	isolateUserConfigHome(t)
	userPath := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte("default_model = \"deepseek-pro\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, err := LockConfigFileEdits(userPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	type answer struct {
		changed bool
		err     error
	}
	done := make(chan answer, 1)
	go func() {
		changed, err := processRoots().MigrateLegacyDeepSeekProtocolUserConfig()
		done <- answer{changed, err}
	}()
	select {
	case got := <-done:
		if got.changed || got.err != nil {
			t.Fatalf("answer = %+v, want nothing to upgrade", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a config with nothing to upgrade waited for the edit lock")
	}
}
