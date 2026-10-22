package tool

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"reasonix/internal/contract/provider"
)

// VideoTool is an optional capability a Tool may implement when its result can
// carry video files for the model to watch natively, beside the text and images
// ImageTool already carries. The files travel as references: the provider reads
// the bytes only when it builds the request that needs them.
type VideoTool interface {
	ExecuteWithVideo(ctx context.Context, args json.RawMessage) (text string, images []string, videos []provider.VideoRef, err error)
}

// VideoLimit is one named ceiling on a file sent natively. The name travels with
// the number so a refusal can say which of them stopped it.
type VideoLimit struct {
	Source string
	Bytes  int64
}

// NativeVideo is what the selected model offers for watching a file natively:
// the protocol a file reaches it on, and every limit that applies to it.
type NativeVideo struct {
	Protocol string
	Model    string
	Limits   []VideoLimit
	// Unavailable is the identity of why the selected model does not take video
	// natively, set when Protocol is empty.
	Unavailable string
	// Addresses is whether the user has let the model hand a public address to the
	// vendor. It is read from the user's own file at the moment of the call.
	Addresses bool
}

// Tightest is the limit that binds: the smallest of those named. No limits reads
// as no ceiling, never as zero bytes allowed.
func (n NativeVideo) Tightest() VideoLimit {
	var best VideoLimit
	for _, l := range n.Limits {
		if l.Bytes > 0 && (best.Bytes == 0 || l.Bytes < best.Bytes) {
			best = l
		}
	}
	return best
}

// NativeVideoSource answers, at the moment a tool runs, whether the selected
// model takes video natively. It is asked per call because the model can change
// between calls.
type NativeVideoSource func() (NativeVideo, bool)

type nativeVideoContextKey struct{}

// WithNativeVideo stamps the source onto a tool execution context.
func WithNativeVideo(ctx context.Context, src NativeVideoSource) context.Context {
	if src == nil {
		return ctx
	}
	return context.WithValue(ctx, nativeVideoContextKey{}, src)
}

// NativeVideoFrom asks the source carried by ctx. ok is false when there is none
// or when the model does not take video natively; the value still says why.
func NativeVideoFrom(ctx context.Context) (NativeVideo, bool) {
	if ctx == nil {
		return NativeVideo{}, false
	}
	src, _ := ctx.Value(nativeVideoContextKey{}).(NativeVideoSource)
	if src == nil {
		return NativeVideo{}, false
	}
	n, ok := src()
	return n, ok && n.Protocol != ""
}

type userAddressesContextKey struct{}

// WithUserAddresses stamps onto a tool execution context how to list the https
// addresses the user wrote in this conversation.
func WithUserAddresses(ctx context.Context, src func() []string) context.Context {
	if src == nil {
		return ctx
	}
	return context.WithValue(ctx, userAddressesContextKey{}, src)
}

// UserAddressesFrom lists the addresses the user wrote, or none when ctx has no source.
func UserAddressesFrom(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	if src, _ := ctx.Value(userAddressesContextKey{}).(func() []string); src != nil {
		return src()
	}
	return nil
}

var addressInText = regexp.MustCompile(`https://[^\s<>"'\x60\]\)]+`)

// AddressesIn lists the https addresses in a user's text, without the sentence
// punctuation that follows one.
func AddressesIn(text string) []string {
	var out []string
	for _, raw := range addressInText.FindAllString(text, -1) {
		out = append(out, strings.TrimRight(raw, ".,;:!?"))
	}
	return out
}
