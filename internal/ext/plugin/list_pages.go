package plugin

import (
	"context"
	"encoding/json"
	"fmt"
)

// maxListPages bounds one catalog read: a server that keeps handing out fresh
// cursors would otherwise hold startup open for as long as it likes.
const maxListPages = 64

// listAllPages reads every page of an MCP list method, following nextCursor
// until the server stops returning one. A cursor seen twice is a server loop,
// and an incomplete catalog is reported rather than served as whole.
func listAllPages[T any](ctx context.Context, c *Client, method, field string) ([]T, error) {
	all, _, err := listAllPagesMeta[T](ctx, c, method, field)
	return all, err
}

// listMeta is what a list response says about how long it holds.
type listMeta struct {
	// ttlMs is the smallest ttlMs any page carried; nil when none did.
	ttlMs *int64
}

// listAllPagesMeta is listAllPages that also reports the freshness hint.
func listAllPagesMeta[T any](ctx context.Context, c *Client, method, field string) ([]T, listMeta, error) {
	var all []T
	var meta listMeta
	seen := map[string]bool{}
	cursor := ""
	for range maxListPages {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		res, err := c.call(ctx, method, params)
		if err != nil {
			return nil, meta, err
		}
		var page map[string]json.RawMessage
		if err := json.Unmarshal(res, &page); err != nil {
			return nil, meta, fmt.Errorf("plugin %q: decode %s: %w", c.name, method, err)
		}
		if raw, ok := page["ttlMs"]; ok {
			var ms int64
			if json.Unmarshal(raw, &ms) == nil && (meta.ttlMs == nil || ms < *meta.ttlMs) {
				meta.ttlMs = &ms
			}
		}
		if raw, ok := page[field]; ok && string(raw) != "null" {
			var items []T
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, meta, fmt.Errorf("plugin %q: decode %s: %w", c.name, method, err)
			}
			all = append(all, items...)
		}
		next := ""
		if raw, ok := page["nextCursor"]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &next); err != nil {
				return nil, meta, fmt.Errorf("plugin %q: decode %s nextCursor: %w", c.name, method, err)
			}
		}
		if next == "" {
			return all, meta, nil
		}
		if seen[next] {
			return nil, meta, &catalogPageError{}
		}
		seen[next] = true
		cursor = next
	}
	return nil, meta, &catalogPageError{limit: maxListPages}
}
