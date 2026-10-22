package plugin

import (
	"context"
	"maps"
	"path/filepath"
	"strings"

	"reasonix/internal/contract/planmode"
)

// metaCwdKey is where a tool call tells a local server the workspace root. The
// key is namespaced as MCP asks of vendor keys in _meta.
const metaCwdKey = "io.reasonix/cwd"

// metaReadOnlyKey tells a local server whether the host is refusing writes right
// now (plan mode), so the server can refuse its own.
const metaReadOnlyKey = "io.reasonix/readOnly"

// metaWritableRootsKey carries the folders the user chose to let tools write
// beside the workspace (allow_write, granted extra roots, --add-dir), so a local
// server can scope its own writes to them. It never carries the workspace, the
// temp directory, a read-forbidden path or anything the host keeps for itself.
const metaWritableRootsKey = "io.reasonix/writableRoots"

// maxMetaWritableRoots bounds what one call tells; the list is configuration a
// person typed, so a long one is a mistake and not something to forward whole.
const maxMetaWritableRoots = 32

// metaWritableRoots is the absolute, de-duplicated folders of roots other than
// the workspace itself, in the order they were configured.
func metaWritableRoots(workspace string, roots []string) []string {
	skip := map[string]bool{}
	if w := strings.TrimSpace(workspace); w != "" {
		skip[filepath.Clean(w)] = true
	}
	var out []string
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		root = filepath.Clean(root)
		if skip[root] {
			continue
		}
		skip[root] = true
		out = append(out, root)
		if len(out) == maxMetaWritableRoots {
			break
		}
	}
	return out
}

// withHostContext adds what a local stdio server may know about the host to a
// tool call. A remote server is told nothing: the workspace path would leave the
// machine, and a server nobody started here has no use for it. The caller's map
// is left alone, and a key the call already carries is never overwritten.
func (c *Client) withHostContext(ctx context.Context, method string, params any) any {
	if method != "tools/call" || canonicalMCPRuntimeTransport(c.spec.Type) != "stdio" {
		return params
	}
	call, ok := params.(map[string]any)
	if !ok {
		return params
	}
	next := maps.Clone(call)
	meta := map[string]any{}
	if existing, ok := call["_meta"].(map[string]any); ok {
		maps.Copy(meta, existing)
	}
	if _, set := meta[metaCwdKey]; !set && strings.TrimSpace(c.spec.WorkspaceRoot) != "" {
		meta[metaCwdKey] = c.spec.WorkspaceRoot
	}
	if _, set := meta[metaWritableRootsKey]; !set {
		if roots := metaWritableRoots(c.spec.WorkspaceRoot, c.spec.WritableRoots); len(roots) > 0 {
			meta[metaWritableRootsKey] = roots
		}
	}
	if _, set := meta[metaReadOnlyKey]; !set {
		meta[metaReadOnlyKey] = planmode.Active(ctx)
	}
	next["_meta"] = meta
	return next
}
