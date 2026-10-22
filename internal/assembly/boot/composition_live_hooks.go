package boot

import "reasonix/internal/ext/hook"

func (b *builder) resolveLiveHookSelection() error {
	binding := b.opts.Composition
	if binding.Digest == "" {
		hooks := hook.Load(hook.LoadOptions{ProjectRoot: b.root, ReasonixHomeDir: b.roots.Home()})
		binding.Resolved.Hooks = hook.ResolveCompositionSelection(binding.Resolved.Hooks, hooks)
	}
	return nil
}
