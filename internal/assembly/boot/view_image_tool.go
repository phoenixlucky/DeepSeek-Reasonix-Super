package boot

import (
	"slices"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/builtin"
)

// addViewImage registers view_image for a session whose model reads images. A
// session on a text-only model is not offered a tool it cannot use, and its tool
// block (and so its cached prompt prefix) is unchanged.
func addViewImage(reg *tool.Registry, cfg *config.Config, entry *config.ProviderEntry, env toolEnvironment, root string) {
	if !config.EffectiveVision(entry) {
		return
	}
	if len(cfg.Tools.Enabled) > 0 && !slices.Contains(cfg.Tools.Enabled, "view_image") {
		return
	}
	reg.Add(builtin.Workspace{
		Dir: root, WriteRoots: env.writeRoots, ReadRoots: env.readRoots, ForbidReadRoots: env.forbidReadRoots, ReadPaths: env.readPaths,
	}.ViewImage())
}

// childViewImage is the image reader a delegated child gets when its own model
// reads images, whatever the parent's does: the parent's registry only holds the
// tool when the parent's model could use it, so a text-only session that hands a
// screenshot to an image model would otherwise leave that child with no way to
// open it. The parent's tool block is not touched.
func childViewImage(cfg *config.Config, env toolEnvironment, root string) func(modelRef string) []tool.Tool {
	if len(cfg.Tools.Enabled) > 0 && !slices.Contains(cfg.Tools.Enabled, "view_image") {
		return nil
	}
	return func(modelRef string) []tool.Tool {
		entry, ok := cfg.ResolveModel(modelRef)
		if !ok || !config.EffectiveVision(entry) {
			return nil
		}
		return []tool.Tool{builtin.Workspace{
			Dir: root, WriteRoots: env.writeRoots, ReadRoots: env.readRoots, ForbidReadRoots: env.forbidReadRoots, ReadPaths: env.readPaths,
		}.ViewImage()}
	}
}
