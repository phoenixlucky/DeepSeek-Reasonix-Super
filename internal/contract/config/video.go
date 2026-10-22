package config

import (
	"strings"

	"reasonix/internal/contract/provider"
)

// The protocols a provider may declare; the wire code that speaks them lives
// with the providers.
const (
	VideoProtocolInlineDataURL = provider.VideoProtocolInlineDataURL
	VideoProtocolMoonshotFile  = provider.VideoProtocolMoonshotFile
	VideoProtocolGeminiInline  = provider.VideoProtocolGeminiInline
	VideoProtocolGeminiFiles   = provider.VideoProtocolGeminiFiles
	VideoProtocolZhipuURL      = provider.VideoProtocolZhipuURL
)

// VideoProtocolsFor lists the protocols a wire can speak: a protocol is a
// property of the endpoint's wire, so a Gemini entry never takes the OpenAI
// ones nor the reverse.
func VideoProtocolsFor(kind string) []string {
	if strings.EqualFold(strings.TrimSpace(kind), "gemini") {
		return []string{VideoProtocolGeminiInline, VideoProtocolGeminiFiles}
	}
	return []string{VideoProtocolInlineDataURL, VideoProtocolMoonshotFile, VideoProtocolZhipuURL}
}

// EffectiveVideo reports how the selected model takes a video file natively. ok
// is false unless the model is declared in video_models and the provider names a
// protocol this build knows; every other case is sampled into frames.
func EffectiveVideo(e *ProviderEntry) (protocol string, ok bool) {
	if e == nil || !e.HasVideoModel(e.Model) {
		return "", false
	}
	for _, known := range VideoProtocolsFor(e.Kind) {
		if e.VideoProtocol == known {
			return known, true
		}
	}
	return "", false
}

// HasVideoModel reports whether model is declared in video_models.
func (e *ProviderEntry) HasVideoModel(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, candidate := range e.VideoModels {
		if strings.EqualFold(strings.TrimSpace(candidate), model) {
			return true
		}
	}
	return false
}
