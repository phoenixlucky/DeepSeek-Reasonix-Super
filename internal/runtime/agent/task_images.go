// task_images.go — images a delegation's own task text names.
package agent

import "context"

type taskImageBinderKey struct{}

// WithTaskImageBinder carries the host's answer to "which images does this task
// text name": a function returning ctx with those images added to the sub-agent
// candidates and the vision routing that goes with them. A follow-up can point at
// an image from earlier, so it is read off the task at the place the task is known.
func WithTaskImageBinder(ctx context.Context, bind func(ctx context.Context, task string) context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if bind == nil {
		return ctx
	}
	return context.WithValue(ctx, taskImageBinderKey{}, bind)
}

// BindTaskImages adds the images task names to ctx's sub-agent candidates. With
// no binder in ctx, or nothing named, ctx is returned as it was.
func BindTaskImages(ctx context.Context, task string) context.Context {
	if ctx == nil {
		return ctx
	}
	if bind, ok := ctx.Value(taskImageBinderKey{}).(func(context.Context, string) context.Context); ok {
		return bind(ctx, task)
	}
	return ctx
}
