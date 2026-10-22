package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"reasonix/internal/base/testenv"
)

// legacyUpgradeFixture writes a legacy-keyed cache entry for a spec whose URL
// carries credential material. The pre-credential-aware normalization hashed
// the raw value while today's redacts it, so the two keys differ and the
// legacy upgrade branch of LoadCachedSchemaForSpec is reachable.
func legacyUpgradeFixture(t *testing.T) (spec Spec, legacy, path string) {
	t.Helper()
	spec = sampleSpec()
	spec.Type = "http"
	spec.Command = ""
	spec.URL = "https://user:pw@example.com/mcp?api_key=secret"
	spec.Env = nil
	current := SchemaCacheKey(spec)
	var ok bool
	legacy, ok = legacySchemaCacheKey(spec)
	if !ok || legacy == current {
		t.Fatalf("fixture: spec has no legacy-key variant (legacy=%q current=%q)", legacy, current)
	}
	path = specCachePath(spec.Name, legacy)
	if path == "" {
		t.Skip("cachePath unavailable in this environment")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveCachedSchema(spec.Name, sampleCachedSchema(legacy)); err != nil {
		t.Fatal(err)
	}
	return spec, legacy, path
}

// denyDirWrites stops new entries in dir until the test ends. A directory's
// read-only attribute does not block writes on Windows, so it takes an ACL there.
func denyDirWrites(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		const everyoneSID = "*S-1-1-0"
		if out, err := exec.Command("icacls", dir, "/deny", everyoneSID+":(W)").CombinedOutput(); err != nil {
			t.Skipf("cannot deny writes to the cache dir: %v\n%s", err, out)
		}
		t.Cleanup(func() { _ = exec.Command("icacls", dir, "/remove:d", everyoneSID).Run() })
		return
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skipf("cannot make cache dir read-only on this platform: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func cacheKeyOnDisk(t *testing.T, path string) string {
	t.Helper()
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var persisted CachedSchema
	if err := json.Unmarshal(onDisk, &persisted); err != nil {
		t.Fatalf("cache is not JSON: %v", err)
	}
	return persisted.CacheKey
}

// TestAuditLegacyCacheUpgradeReportsOnlySuccess: the legacy-key upgrade path
// cannot rewrite a read-only cache dir, so the caller must be handed the key
// the file still holds (internal/ext/plugin/cache.go:166). Otherwise the caller
// reads a different key back after a restart and boot pins the wrong MCP tool
// identity (internal/assembly/boot/mcp_registration.go:76).
func TestAuditLegacyCacheUpgradeReportsOnlySuccess(t *testing.T) {
	cacheRoot := testenv.TempDir(t)
	t.Setenv("REASONIX_CACHE_HOME", cacheRoot)
	spec, legacy, p := legacyUpgradeFixture(t)

	// Make every replacement in that directory fail while reads still work.
	denyDirWrites(t, filepath.Dir(p))

	got, ok := LoadCachedSchemaForSpec(spec)
	if !ok {
		t.Skip("environment refused the read too; test cannot isolate the write failure")
	}
	persisted := cacheKeyOnDisk(t, p)
	if persisted != legacy {
		t.Fatalf("fixture: on-disk key = %q, want the legacy %q (the read-only dir should have blocked the upgrade write)", persisted, legacy)
	}
	// The caller cannot observe the failed write, so the object it gets must
	// carry the key the file actually holds.
	if got.CacheKey != persisted {
		t.Fatalf("LoadCachedSchemaForSpec returned ok=true with CacheKey=%q, but the on-disk key is %q; "+
			"SaveCachedSchema failed (read-only cache dir) at cache.go:166 and its error was discarded, so the caller was handed a key that disagrees with the file",
			got.CacheKey, persisted)
	}
	if got.CacheKey != legacy {
		t.Fatalf("failure case reported key %q, want the legacy key %q: the upgrade did not persist, so the caller must get the object as it sits on disk", got.CacheKey, legacy)
	}
	// Re-reading through the returned key agrees: the caller can round-trip it.
	if cs, ok := LoadCachedSchema(spec.Name, got.CacheKey); !ok || cs.CacheKey != got.CacheKey {
		t.Fatalf("re-reading the cache with the returned key %q disagrees with the object handed back", got.CacheKey)
	}
}

// TestLegacyCacheUpgradePersistsWhenWritable is the control for the test
// above: with a writable cache dir the same upgrade does reach disk and the
// returned key equals the on-disk key, so a fix cannot "succeed" by never
// upgrading.
func TestLegacyCacheUpgradePersistsWhenWritable(t *testing.T) {
	cacheRoot := testenv.TempDir(t)
	t.Setenv("REASONIX_CACHE_HOME", cacheRoot)
	spec, legacy, p := legacyUpgradeFixture(t)
	current := SchemaCacheKey(spec)

	got, ok := LoadCachedSchemaForSpec(spec)
	if !ok {
		t.Fatal("legacy cache entry did not load from a writable directory")
	}
	_ = p
	persisted := cacheKeyOnDisk(t, specCachePath(spec.Name, current))
	if persisted != current {
		t.Fatalf("on-disk key = %q, want the persisted upgrade %q (legacy was %q)", persisted, current, legacy)
	}
	if got.CacheKey != persisted {
		t.Fatalf("returned key = %q, want the on-disk %q", got.CacheKey, persisted)
	}
	if _, ok := LoadCachedSchema(spec.Name, current); !ok {
		t.Fatal("upgrade was reported as persisted but a current-key load misses")
	}
}
