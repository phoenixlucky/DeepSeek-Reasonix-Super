package boot

import (
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/builtin"
)

// videoProtocolOf is the protocol a video file reaches the selected model on, or
// "" when the model is not declared to watch video: an endpoint never sends a
// video it was not told its model can take.
func videoProtocolOf(e *config.ProviderEntry) string {
	protocol, _ := config.EffectiveVideo(e)
	return protocol
}

// nativeVideoSource answers, for the model this build selected, whether a tool
// may hand it a video file and under which limits. The limits are each named: the
// protocol's documented one, and the user's own when they lowered it.
func nativeVideoSource(entry *config.ProviderEntry, cfg *config.Config) tool.NativeVideoSource {
	if entry == nil {
		return nil
	}
	protocol, ok := config.EffectiveVideo(entry)
	model := entry.Model
	unavailable := builtin.CodeVideoNativeUnsupported
	if entry.HasVideoModel(model) {
		unavailable = builtin.CodeVideoNativeNoProtocol
	}
	return func() (tool.NativeVideo, bool) {
		if !ok {
			return tool.NativeVideo{Unavailable: unavailable}, false
		}
		source, bytes := provider.VideoProtocolLimit(protocol)
		limits := []tool.VideoLimit{{Source: source, Bytes: bytes}}
		if cfg != nil && cfg.Tools.VideoNativeMaxBytes > 0 {
			limits = append(limits, tool.VideoLimit{Source: "tools.video_native_max_bytes", Bytes: cfg.Tools.VideoNativeMaxBytes})
		}
		// The switch is read from the user's own file at each call, so a change
		// the user confirms applies to the next one.
		addresses := config.LoadForEdit(config.UserConfigPath()).Tools.VideoURLPassthrough
		return tool.NativeVideo{Protocol: protocol, Model: model, Limits: limits, Addresses: addresses}, true
	}
}
