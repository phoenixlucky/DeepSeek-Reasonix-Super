package control

import (
	"context"
	"errors"

	"reasonix/internal/ext/hook"
)

// A plugin package owns its hooks; editing them here would be overwritten the
// next time the package updates.
var errPluginHooksReadOnly = errors.New("plugin hooks belong to their package; disable the plugin instead")

// InspectHooks returns the configured hooks with their diagnostics — malformed
// settings files, unknown event names, uncompilable matchers. A hooks editor has
// to show those while editing; finding them at run time means finding them
// mid-task, which is the worst moment.
func (c *Controller) InspectHooks() hook.Inspection {
	return hook.Inspect(c.hookLoadOptions())
}

func (c *Controller) hookLoadOptions() hook.LoadOptions {
	return hook.LoadOptions{ProjectRoot: c.workspaceRoot, ReasonixHomeDir: c.PresetStore().Home}
}

// SaveHooks writes one scope's hooks and swaps them into the live session, so a
// rule takes effect on the next tool call rather than the next launch. Plugin
// hooks are not writable here: they belong to their package.
func (c *Controller) SaveHooks(scope hook.Scope, settings hook.Settings) error {
	if scope == hook.ScopePlugin {
		return errPluginHooksReadOnly
	}
	opts := c.hookLoadOptions()
	if err := hook.SaveFor(scope, opts, settings); err != nil {
		return err
	}
	// Saving from the editor is the person choosing these hooks as written.
	if scope == hook.ScopeProject {
		if err := hook.ApproveSavedProjectHooks(opts, settings); err != nil {
			return err
		}
	}
	binding := c.CompositionBinding()
	if binding == nil {
		c.hooks.Replace(hook.Load(opts))
	} else if binding.Live() {
		c.hooks.Replace(hook.CompositionHooks(hook.Load(opts), binding.Resolved))
	} else {
		c.hooks.ReplacePolicyGuards(hook.Load(opts))
	}
	return nil
}

// DryRunHook really executes one hook against a representative payload. It is
// the difference between writing a hook and knowing it works, and the only way
// to learn what a given exit code does on a given event without provoking it
// during real work.
func (c *Controller) DryRunHook(ctx context.Context, cfg hook.HookConfig, event hook.Event) (hook.DryRunResult, error) {
	cwd := c.workspaceRoot
	if cwd == "" {
		cwd = c.SessionDir()
	}
	return hook.DryRun(ctx, cfg, event, cwd, c.hooks.Spawner())
}

// HookSettingsPath is where a scope's rules are stored. The editor names it,
// because a project file is shared with everyone who clones the repository and
// a global one is not.
func (c *Controller) HookSettingsPath(scope hook.Scope) string {
	return hook.SettingsPathFor(scope, c.hookLoadOptions())
}
