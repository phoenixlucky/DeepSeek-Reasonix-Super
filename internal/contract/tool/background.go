package tool

import (
	"context"
	"io"
)

// BackgroundStarter starts work that outlives the call asking for it and
// returns the host-minted id the model addresses it by. It is what lets a tool
// in a layer above the job registry hand a long wait to it.
type BackgroundStarter func(kind, label string, run func(ctx context.Context, out io.Writer) (string, error)) string

type backgroundKey struct{}

// WithBackgroundStarter stamps ctx with the starter the agent builds for a call.
func WithBackgroundStarter(ctx context.Context, start BackgroundStarter) context.Context {
	return context.WithValue(ctx, backgroundKey{}, start)
}

// BackgroundStarterFrom returns the starter, if the call carries one.
func BackgroundStarterFrom(ctx context.Context) (BackgroundStarter, bool) {
	start, ok := ctx.Value(backgroundKey{}).(BackgroundStarter)
	return start, ok && start != nil
}
