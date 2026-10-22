// Package-internal cache for MCP handshake results. The handshake (initialize +
// listTools, plus optional listPrompts/listResources) costs hundreds of ms to a
// few seconds per server on cold start. We persist the tool schema +
// capabilities under the user cache dir keyed by load-bearing, non-secret Spec
// fields, so the next launch can register tools optimistically
// without waiting for the network/subprocess. Caching is purely an
// optimisation: a cache miss (missing dir, bad JSON, key mismatch) silently
// degrades to a fresh handshake.
package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"reasonix/internal/base/fileutil"
	fileencoding "reasonix/internal/base/fileutil/encoding"
	"reasonix/internal/base/secrets"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
)

// cacheableToolsOf extracts the persistable subset of remote tools so Start()
// can hand them to SaveCachedSchema. Non-remote tools are skipped — Start
// only feeds remote ones at the call site, but the type-assert is defensive.
func cacheableToolsOf(tools []tool.Tool) []CachedTool {
	out := make([]CachedTool, 0, len(tools))
	for _, t := range tools {
		rt, ok := t.(*remoteTool)
		if !ok {
			continue
		}
		declaredReadOnly, _, destructive := rt.securitySnapshot()
		out = append(out, CachedTool{
			Name:         rt.rawName,
			Description:  rt.desc,
			Schema:       rt.schema,
			OutputSchema: rt.outputSchema,
			ReadOnly:     declaredReadOnly,
			Destructive:  destructive,
		})
	}
	return out
}

// cacheVersion bumps whenever CachedSchema shape changes incompatibly. Old
// files with a smaller version are treated as a miss so a stale layout never
// crashes a reader.
const cacheVersion = 2

// CachedSchema is the persisted snapshot of one server's handshake result.
// CacheKey gates reuse — Capabilities/Tools are only trusted when the
// caller's expectedKey (from SchemaCacheKey of the current Spec) matches,
// so renaming env vars or swapping a command never serves stale tools.
type CachedSchema struct {
	Version int `json:"version"`
	// Keep the historical JSON key so older versions can reuse the same
	// best-effort cache during rolling upgrades.
	CacheKey     string          `json:"spec_hash"`
	Capabilities map[string]bool `json:"capabilities"`
	// Instructions is the server's own description, kept so a configured but
	// unconnected server can still say what it is for. Optional by design: an
	// older cache file simply has none.
	Instructions  string       `json:"instructions,omitempty"`
	Tools         []CachedTool `json:"tools"`
	LastValidated time.Time    `json:"last_validated"`
}

// CachedTool mirrors the subset of an MCP tool definition we need to register
// a placeholder before the real handshake completes: Name (raw, server-local),
// Description (model-visible), Schema (raw JSON for input validation),
// ReadOnly and Destructive (drive Plan/read-only safety classification).
type CachedTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Schema       json.RawMessage `json:"schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
	ReadOnly     bool            `json:"read_only"`
	Destructive  bool            `json:"destructive,omitempty"`
}

// SchemaCacheKey hashes the load-bearing, non-secret parts of a Spec. Secret
// values in env and headers are intentionally excluded: credential rotation
// must not leak through a stable digest or force an unrelated trust review.
// Their sorted key names remain identity-bearing, so adding/removing a runtime
// input still invalidates the cached schema.
func SchemaCacheKey(s Spec) string {
	return schemaCacheKeyForURL(s, normalizeIdentityURL(s.URL))
}

// legacySchemaCacheKey recomputes the cache key with the
// pre-credential-aware URL normalization, or ("", false) when it cannot
// differ. LoadCachedSchemaForSpec uses it to upgrade old cache entries in
// place; remove together with legacyNormalizeIdentityURL.
func legacySchemaCacheKey(s Spec) (string, bool) {
	legacyURL := legacyNormalizeIdentityURL(s.URL)
	if strings.TrimSpace(s.URL) == "" || legacyURL == normalizeIdentityURL(s.URL) {
		return "", false
	}
	return schemaCacheKeyForURL(s, legacyURL), true
}

func schemaCacheKeyForURL(s Spec, urlValue string) string {
	h := sha256.New()
	writeField(h, "name", s.Name)
	writeField(h, "type", s.Type)
	writeField(h, "command", s.Command)
	writeField(h, "url", urlValue)
	writeField(h, "dir", s.Dir)
	for _, a := range s.Args {
		writeField(h, "arg", a)
	}
	for _, name := range disabledToolNames(s.DisabledTools) {
		writeField(h, "disabled_tool", name)
	}
	writeKeys(h, "env", s.Env)
	writeKeys(h, "headers", s.Headers)
	return hex.EncodeToString(h.Sum(nil))
}

// LoadCachedSchema returns the cached schema for name iff it exists, parses,
// and matches expectedKey. Any error → (nil, false): cache is best-effort,
// a corrupt file just means we re-handshake. Returning an error here would
// only invite callers to log it on every launch — silence is intentional.
func LoadCachedSchema(name, expectedKey string) (*CachedSchema, bool) {
	cs, ok, keyOK := LoadCachedSchemaAny(name, expectedKey)
	if !ok || !keyOK {
		return nil, false
	}
	return cs, true
}

// LoadCachedSchemaForSpec returns a usable enabled-tool snapshot matching the
// current spec. Invalid model-visible names are a cache miss. A matching legacy
// URL key is upgraded in place without changing the tool definitions; if that
// rewrite fails, the hit keeps the legacy key the file still holds, so no
// caller is handed a key that disagrees with the disk.
func LoadCachedSchemaForSpec(s Spec) (*CachedSchema, bool) {
	current := SchemaCacheKey(s)
	if cs, ok := LoadCachedSchema(s.Name, current); ok {
		cs.Tools = s.EnabledCachedTools(cs.Tools)
		if !cachedToolNamesUsable(s, cs.Tools) {
			return nil, false
		}
		return cs, true
	}
	legacy, ok := legacySchemaCacheKey(s)
	if !ok {
		return nil, false
	}
	cs, ok := LoadCachedSchema(s.Name, legacy)
	if !ok {
		return nil, false
	}
	cs.Tools = s.EnabledCachedTools(cs.Tools)
	if !cachedToolNamesUsable(s, cs.Tools) {
		return nil, false
	}
	// Claim the upgraded key only once it reached the disk: the caller
	// cannot see this write, so it must not get a key the file lacks.
	upgraded := *cs
	upgraded.CacheKey = current
	if err := SaveCachedSchema(s.Name, upgraded); err == nil {
		cs.CacheKey = current
	}
	return cs, true
}

func cachedToolNamesUsable(s Spec, tools []CachedTool) bool {
	if err := validateCachedToolNames(s, tools); err != nil {
		slog.Warn("plugin cache: catalog rejected", "server", s.Name, "err", secrets.DiagnosticError(err))
		return false
	}
	return true
}

// LoadCachedSchemaAny returns the cached schema regardless of cache-key match,
// plus whether the key matched expectedKey. Catalog building uses it so a
// mismatched cache can still surface tools as stale candidates;
// execution paths must keep using LoadCachedSchema, which refuses mismatches.
func LoadCachedSchemaAny(name, expectedKey string) (cs *CachedSchema, ok bool, keyOK bool) {
	for _, p := range []string{specCachePath(name, expectedKey), cachePath(name)} {
		if out, found := readCachedSchema(p); found {
			return out, true, out.CacheKey == expectedKey
		}
	}
	// No entry for this spec: a sibling for the same name still shows what the
	// server last offered, as a stale candidate (keyOK stays false).
	if out, found := readCachedSchema(newestSiblingCachePath(name)); found {
		return out, true, false
	}
	return nil, false, false
}

// readCachedSchema reads and validates one cache file; "" or a bad file is a miss.
func readCachedSchema(p string) (*CachedSchema, bool) {
	if p == "" {
		return nil, false
	}
	b, err := fileencoding.ReadFileUTF8(p)
	if err != nil {
		return nil, false
	}
	var out CachedSchema
	if err := json.Unmarshal(b, &out); err != nil || out.Version != cacheVersion {
		return nil, false
	}
	out.Tools = filterValidCachedTools(out.Tools)
	return &out, true
}

func filterValidCachedTools(tools []CachedTool) []CachedTool {
	out := make([]CachedTool, 0, len(tools))
	for _, t := range tools {
		schema, err := normalizeAndValidateToolSchema(t.Schema)
		if err != nil {
			continue
		}
		t.Schema = schema
		out = append(out, t)
	}
	return out
}

// SaveCachedSchema atomically writes cs under name. Best-effort: an error is
// logged at debug level and returned. The shared replacement helper preserves
// overwrite semantics on Windows as well as crash safety on Unix.
func SaveCachedSchema(name string, cs CachedSchema) error {
	p := specCachePath(name, cs.CacheKey)
	if p == "" {
		p = cachePath(name)
	}
	if p == "" {
		return nil
	}
	cs.Version = cacheVersion
	if cs.LastValidated.IsZero() {
		cs.LastValidated = time.Now().UTC()
	}
	b, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		slog.Debug("plugin cache: marshal", "name", name, "err", err)
		return err
	}
	if err := fileutil.AtomicWriteFile(p, b, 0o600); err != nil {
		slog.Debug("plugin cache: atomic write", "name", name, "err", err)
		return err
	}
	pruneSpecCaches(name, p)
	return nil
}

// maxSpecCachesPerName bounds the entries kept for one server name. Entries are
// per spec, so projects that declare one name differently each keep theirs; the
// bound only stops a name that changes its launch spec forever from growing the
// directory, and a pruned entry costs one handshake when its spec returns.
const maxSpecCachesPerName = 8

// specCachePath is the entry for one server name and one spec key:
// "<cache>/mcp/<slug(name)>.<key prefix>.json". The slug holds no dot, so a name
// never matches another name's entries. "" when there is no cache directory or
// no key to tell specs apart.
func specCachePath(name, key string) string {
	tag := cacheKeyTag(key)
	base := config.CacheDir()
	if tag == "" || base == "" {
		return ""
	}
	return filepath.Join(base, "mcp", slug(name)+"."+tag+".json")
}

func cacheKeyTag(key string) string {
	tag := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' {
			return r
		}
		return -1
	}, strings.ToLower(key))
	return tag[:min(len(tag), 16)]
}

// siblingCachePaths lists every per-spec entry of name, newest first.
func siblingCachePaths(name string) []string {
	base := config.CacheDir()
	if base == "" {
		return nil
	}
	dir := filepath.Join(base, "mcp")
	entries, _ := os.ReadDir(dir)
	prefix := slug(name) + "."
	type dated struct {
		path string
		at   time.Time
	}
	var list []dated
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".json") {
			continue
		}
		if info, err := e.Info(); err == nil {
			list = append(list, dated{filepath.Join(dir, n), info.ModTime()})
		}
	}
	slices.SortFunc(list, func(a, b dated) int { return b.at.Compare(a.at) })
	out := make([]string, len(list))
	for i, d := range list {
		out[i] = d.path
	}
	return out
}

func newestSiblingCachePath(name string) string {
	if all := siblingCachePaths(name); len(all) > 0 {
		return all[0]
	}
	return ""
}

// pruneSpecCaches drops the oldest entries of name past the bound, never the one
// just written.
func pruneSpecCaches(name, keep string) {
	all := siblingCachePaths(name)
	for i, p := range all {
		if i >= maxSpecCachesPerName && p != keep {
			_ = os.Remove(p)
		}
	}
}

// cachePath returns "<config.CacheDir()>/mcp/<slug(name)>.json". Returns ""
// when CacheDir is unavailable (no-op caching).
func cachePath(name string) string {
	base := config.CacheDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "mcp", slug(name)+".json")
}

// slugReplace strips characters that aren't safe in a filename across the
// OSes we target. We lowercase first so the slug is stable regardless of
// the user's display capitalisation.
var slugReplace = regexp.MustCompile(`[^a-z0-9_-]+`)

// windowsReservedDeviceNames are DOS device names Windows reserves as file
// stems (with or without an extension), matched case-insensitively.
var windowsReservedDeviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// slug sanitises name for use as a filename. Names changed by sanitization —
// and Windows-reserved device stems such as "con" or "com1", which would name
// a device rather than a file — get a strong suffix so confusable names cannot
// make one MCP server consume another server's cached schemas, stats, or
// private state directory. Ordinary safe names stay byte-identical.
func slug(name string) string {
	s := slugReplace.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "_"
	}
	if s != name || windowsReservedDeviceNames[s] {
		sum := sha256.Sum256([]byte(name))
		s += "-" + hex.EncodeToString(sum[:6])
	}
	return s
}

// writeField feeds a single tagged field into h with explicit separators so
// the boundary between (key, value) and the next field can't collide via
// concatenation (e.g. "command" + "foo" vs "comm" + "andfoo").
func writeField(h io.Writer, key, val string) {
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(val))
	_, _ = h.Write([]byte{1})
}

// writeKeys hashes only sorted map keys, so Go's randomised iteration order
// cannot perturb the non-secret identity digest.
func writeKeys(h io.Writer, key string, m map[string]string) {
	if len(m) == 0 {
		writeField(h, key, "")
		return
	}
	keys := slices.Sorted(maps.Keys(m))
	for _, k := range keys {
		writeField(h, key+"."+k, "present")
	}
}

// saveHandshakeSchema persists what the handshake just learned about a server:
// what it can do, what it said it is for, and the tools it listed.
func (c *Client) saveHandshakeSchema(spec Spec, tools []tool.Tool) {
	_ = SaveCachedSchema(spec.Name, CachedSchema{
		CacheKey:     SchemaCacheKey(spec),
		Capabilities: c.advertisedCapabilities(),
		Instructions: c.instructions,
		Tools:        cacheableToolsOf(tools),
	})
}

// advertisedCapabilities is what the server declared at initialize, in the form
// the schema cache keeps it.
func (c *Client) advertisedCapabilities() map[string]bool {
	return map[string]bool{"tools": c.hasTools, "prompts": c.hasPrompts, "resources": c.hasResources}
}

// cachedCapabilities is advertisedCapabilities of the named live server. A
// server with no live client keeps only what its tool list says.
func (h *Host) cachedCapabilities(server string, haveTools bool) map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c.name == server {
			return c.advertisedCapabilities()
		}
	}
	return map[string]bool{"tools": haveTools}
}
