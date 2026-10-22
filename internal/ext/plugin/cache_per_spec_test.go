package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func specArgs(name string, args ...string) Spec {
	return Spec{Name: name, Type: "stdio", Command: "sh", Args: args}
}

func saveFor(t *testing.T, s Spec, tool string) {
	t.Helper()
	if err := SaveCachedSchema(s.Name, CachedSchema{CacheKey: SchemaCacheKey(s), Tools: []CachedTool{{Name: tool}}}); err != nil {
		t.Fatal(err)
	}
}

// Two servers with one name and different launch specs each keep their own
// entry: saving the second does not evict the first.
func TestSameNameDifferentSpecsKeepTheirOwnCacheEntries(t *testing.T) {
	redirectCache(t)
	a, b := specArgs("docs", "./a.sh"), specArgs("docs", "./b.sh")
	saveFor(t, a, "x")
	saveFor(t, b, "y")
	for name, tc := range map[string]struct {
		spec Spec
		tool string
	}{"a": {a, "x"}, "b": {b, "y"}} {
		cs, ok := LoadCachedSchemaForSpec(tc.spec)
		if !ok || len(cs.Tools) != 1 || cs.Tools[0].Name != tc.tool {
			t.Errorf("spec %s: hit=%v tools=%+v, want its own entry (%s)", name, ok, cs, tc.tool)
		}
	}
}

// An entry written before entries were per spec (one file per name) still reads
// for the spec whose key it holds, and for no other.
func TestAnOldNameOnlyEntryStillReadsForItsOwnSpec(t *testing.T) {
	redirectCache(t)
	a, other := specArgs("docs", "./a.sh"), specArgs("docs", "./other.sh")
	p := cachePath("docs")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	old := fmt.Sprintf(`{"version":%d,"spec_hash":%q,"capabilities":{},"tools":[{"name":"echo","schema":{"type":"object"}}],"last_validated":"2026-01-01T00:00:00Z"}`, cacheVersion, SchemaCacheKey(a))
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadCachedSchemaForSpec(a); !ok {
		t.Error("the old entry no longer reads for the spec it was written for")
	}
	if _, ok := LoadCachedSchemaForSpec(other); ok {
		t.Error("the old entry was used for a different spec")
	}
}

// When no entry matches the spec, the catalog view still sees what the server
// last offered, marked as not matching.
func TestACatalogViewFallsBackToASiblingEntryAsStale(t *testing.T) {
	redirectCache(t)
	saved, asked := specArgs("docs", "./a.sh"), specArgs("docs", "./changed.sh")
	saveFor(t, saved, "x")
	cs, ok, keyOK := LoadCachedSchemaAny("docs", SchemaCacheKey(asked))
	if !ok || keyOK || len(cs.Tools) != 1 {
		t.Fatalf("Any = %+v ok=%v keyOK=%v, want the sibling as a stale candidate", cs, ok, keyOK)
	}
	if _, ok := LoadCachedSchema("docs", SchemaCacheKey(asked)); ok {
		t.Fatal("an execution path was handed a sibling's entry")
	}
}

// The entries kept for one name are bounded, oldest dropped, and pruning one
// name never touches a name that merely begins with it.
func TestTheEntriesOfOneNameAreBoundedWithoutTouchingAnotherName(t *testing.T) {
	redirectCache(t)
	neighbour := specArgs("docs-extra", "./n.sh")
	saveFor(t, neighbour, "n")
	var specs []Spec
	for i := range maxSpecCachesPerName + 3 {
		s := specArgs("docs", fmt.Sprintf("./%d.sh", i))
		saveFor(t, s, "t")
		past := time.Now().Add(time.Duration(i-100) * time.Minute)
		_ = os.Chtimes(specCachePath("docs", SchemaCacheKey(s)), past, past)
		specs = append(specs, s)
	}
	last := specArgs("docs", "./last.sh")
	saveFor(t, last, "t")
	if got := len(siblingCachePaths("docs")); got != maxSpecCachesPerName {
		t.Fatalf("%d entries kept for the name, want %d", got, maxSpecCachesPerName)
	}
	if _, ok := LoadCachedSchemaForSpec(last); !ok {
		t.Error("the entry just written was pruned")
	}
	if _, ok := LoadCachedSchemaForSpec(specs[0]); ok {
		t.Error("the oldest entry survived the bound")
	}
	if _, ok := LoadCachedSchemaForSpec(neighbour); !ok {
		t.Error("pruning one name removed an entry of a name that begins with it")
	}
}
