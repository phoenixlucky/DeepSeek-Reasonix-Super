package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func videoEntry(model string, models []string, protocol string) *ProviderEntry {
	return &ProviderEntry{Name: "gw", Kind: "openai", BaseURL: "https://gw.example/v1", Model: model, Models: []string{model}, VideoModels: models, VideoProtocol: protocol}
}

// A model reads video natively only when someone declared the model and named
// the wire the endpoint takes it on: the declaration says what reads video, the
// protocol says how a file reaches it, and neither is guessed from a name.
func TestEffectiveVideoNeedsADeclaredModelAndAKnownProtocol(t *testing.T) {
	cases := []struct {
		name  string
		entry *ProviderEntry
		want  string
	}{
		{"declared with a known protocol", videoEntry("qwen-vl", []string{"qwen-vl"}, VideoProtocolInlineDataURL), VideoProtocolInlineDataURL},
		{"the upload protocol", videoEntry("kimi-k3", []string{"kimi-k3"}, VideoProtocolMoonshotFile), VideoProtocolMoonshotFile},
		{"case-insensitive model match like vision", videoEntry("Qwen-VL", []string{"qwen-vl"}, VideoProtocolInlineDataURL), VideoProtocolInlineDataURL},
		{"declared but no protocol", videoEntry("qwen-vl", []string{"qwen-vl"}, ""), ""},
		{"declared with a protocol nobody knows", videoEntry("qwen-vl", []string{"qwen-vl"}, "carrier-pigeon"), ""},
		{"a protocol without the model declared", videoEntry("qwen-vl", nil, VideoProtocolInlineDataURL), ""},
		{"another model declared", videoEntry("qwen-vl", []string{"other"}, VideoProtocolInlineDataURL), ""},
		{"a name that looks like video is not a declaration", videoEntry("qwen-vl-video", nil, VideoProtocolInlineDataURL), ""},
	}
	for _, c := range cases {
		got, ok := EffectiveVideo(c.entry)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s: EffectiveVideo = %q,%v, want %q", c.name, got, ok, c.want)
		}
	}
}

// nil is nobody having said; an explicit empty list is a choice. The two must
// survive a render and a parse, or saving the file would turn a refusal back
// into a silence.
func TestVideoModelsKeepNilApartFromAnExplicitEmptyList(t *testing.T) {
	orig := Default()
	orig.Providers = []ProviderEntry{
		{Name: "said", Kind: "openai", BaseURL: "https://a.example/v1", Models: []string{"m1"}, Default: "m1", APIKeyEnv: "A_KEY", VideoModels: []string{"m1"}, VideoProtocol: VideoProtocolInlineDataURL},
		{Name: "refused", Kind: "openai", BaseURL: "https://b.example/v1", Models: []string{"m1"}, Default: "m1", APIKeyEnv: "B_KEY", VideoModels: []string{}},
		{Name: "silent", Kind: "openai", BaseURL: "https://c.example/v1", Models: []string{"m1"}, Default: "m1", APIKeyEnv: "C_KEY"},
	}
	rendered := RenderTOMLForScope(orig, RenderScopeFull)
	for _, want := range []string{`video_models = ["m1"]`, `video_models = []`, `video_protocol = "inline_data_url"`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered TOML missing %q:\n%s", want, rendered)
		}
	}
	var got Config
	if _, err := toml.Decode(rendered, &got); err != nil {
		t.Fatal(err)
	}
	said, _ := got.Provider("said")
	refused, _ := got.Provider("refused")
	silent, _ := got.Provider("silent")
	if !reflect.DeepEqual(said.VideoModels, []string{"m1"}) || said.VideoProtocol != VideoProtocolInlineDataURL {
		t.Fatalf("declared entry after round trip = %#v %q", said.VideoModels, said.VideoProtocol)
	}
	if refused.VideoModels == nil || len(refused.VideoModels) != 0 {
		t.Fatalf("explicit empty list after round trip = %#v", refused.VideoModels)
	}
	if silent.VideoModels != nil {
		t.Fatalf("an entry that never said anything gained %#v", silent.VideoModels)
	}
}

func TestRenamingAModelFollowsItIntoVideoModels(t *testing.T) {
	p := ProviderEntry{Name: "gw", Models: []string{"old"}, Default: "old", VideoModels: []string{"old"}}
	if !renameProviderModel(&p, "old", "new") || !reflect.DeepEqual(p.VideoModels, []string{"new"}) {
		t.Fatalf("video_models after a rename = %v, want [new]", p.VideoModels)
	}
}

func TestAVideoProtocolBelongsToTheWireThatSpeaksIt(t *testing.T) {
	for _, tc := range []struct {
		kind, protocol string
		want           bool
	}{
		{"gemini", VideoProtocolGeminiInline, true},
		{"gemini", VideoProtocolGeminiFiles, true},
		{"gemini", VideoProtocolInlineDataURL, false},
		{"openai", VideoProtocolInlineDataURL, true},
		{"openai", VideoProtocolMoonshotFile, true},
		{"openai", VideoProtocolGeminiInline, false},
		{"anthropic", VideoProtocolGeminiFiles, false},
	} {
		e := &ProviderEntry{Kind: tc.kind, Model: "m", VideoModels: []string{"m"}, VideoProtocol: tc.protocol}
		if _, ok := EffectiveVideo(e); ok != tc.want {
			t.Errorf("%s / %s: native = %v, want %v", tc.kind, tc.protocol, ok, tc.want)
		}
	}
}
