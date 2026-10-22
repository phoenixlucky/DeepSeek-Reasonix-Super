package permission

import (
	"context"
	"encoding/json"
)

type originKey struct{}

// WithOrigin marks ctx with where the text in a call's arguments came from, as the
// host recorded it (for example "web:example.com"). The gate reads it; nothing here
// decides what the label means.
func WithOrigin(ctx context.Context, label string) context.Context {
	if label == "" {
		return ctx
	}
	return context.WithValue(ctx, originKey{}, label)
}

// OriginFrom returns the label WithOrigin set, or "".
func OriginFrom(ctx context.Context) string {
	s, _ := ctx.Value(originKey{}).(string)
	return s
}

// WithAskOnOrigin returns a copy of p with the origin question switched on or off.
func (p Policy) WithAskOnOrigin(on bool) Policy {
	p.AskOnOrigin = on
	return p
}

// allowedOnlyByMode reports that the call runs because the fallback Mode says so:
// with the fallback turned to Ask, no rule and no read classification allows it.
func (p Policy) allowedOnlyByMode(toolName string, readOnly bool, args json.RawMessage) bool {
	p.Mode = Ask
	return p.Decide(toolName, readOnly, args) != Allow
}
