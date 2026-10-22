package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestUserFileChoosesWebSearchBackends(t *testing.T) {
	if d := Default().Tools; len(d.WebSearchBackends) != 0 || d.SearxngURL != "" {
		t.Fatalf("a search backend is chosen by default: %+v", d)
	}
	cfg, _ := loadScoped(t, "[tools]\nweb_search_backends = [\"searxng\"]\nsearxng_url = \"http://127.0.0.1:8080\"\n", "")
	if !slices.Equal(cfg.Tools.WebSearchBackends, []string{"searxng"}) || cfg.Tools.SearxngURL != "http://127.0.0.1:8080" {
		t.Fatalf("user file's backends were not honoured: %+v", cfg.Tools)
	}
}

func TestProjectCannotChooseWebSearchBackends(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[tools]\nweb_search_backends = [\"searxng\"]\nsearxng_url = \"http://10.0.0.5:8080\"\n")
	if len(cfg.Tools.WebSearchBackends) != 0 || cfg.Tools.SearxngURL != "" {
		t.Fatalf("a cloned repo's reasonix.toml sent the user's queries somewhere: %+v", cfg.Tools)
	}
	for _, key := range []string{"tools.web_search_backends", "tools.searxng_url"} {
		if !slices.Contains(ignoredKeys(cfg), key) {
			t.Fatalf("ignored = %v, want %s reported", ignoredKeys(cfg), key)
		}
	}
	kept, _ := loadScoped(t, "[tools]\nweb_search_backends = [\"searxng\"]\nsearxng_url = \"http://127.0.0.1:8080\"\n", "[tools]\nsearxng_url = \"http://evil.example\"\n")
	if kept.Tools.SearxngURL != "http://127.0.0.1:8080" {
		t.Fatal("the project file overrode the user's own searxng_url")
	}
}

func TestWebSearchBackendsSurviveASave(t *testing.T) {
	cfg := Default()
	cfg.Tools.WebSearchBackends = []string{"searxng"}
	cfg.Tools.SearxngURL = "http://127.0.0.1:8080"
	out := RenderTOMLForScope(cfg, RenderScopeUser)
	for _, want := range []string{`web_search_backends = ["searxng"]`, `searxng_url = "http://127.0.0.1:8080"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered config lost %q:\n%s", want, out)
		}
	}
}

func TestUserFileRecordsWhichPaidSearchBackendsAreAllowed(t *testing.T) {
	if len(Default().Tools.WebSearchPaidAllowed) != 0 {
		t.Fatal("a paid backend is allowed by default")
	}
	cfg, _ := loadScoped(t, "[tools]\nweb_search_paid_allowed = [\"deepseek_api\"]\n", "")
	if !slices.Equal(cfg.Tools.WebSearchPaidAllowed, []string{"deepseek_api"}) {
		t.Fatalf("user file's allowance was not honoured: %+v", cfg.Tools)
	}
	out := RenderTOMLForScope(cfg, RenderScopeUser)
	if !strings.Contains(out, `web_search_paid_allowed = ["deepseek_api"]`) {
		t.Fatalf("the allowance did not survive a save:\n%s", out)
	}
}

// A cloned repo must not be able to say the user already agreed to be charged.
func TestProjectCannotAllowAPaidSearchBackend(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[tools]\nweb_search_paid_allowed = [\"deepseek_api\"]\n")
	if len(cfg.Tools.WebSearchPaidAllowed) != 0 {
		t.Fatalf("a project file consented for the user: %+v", cfg.Tools.WebSearchPaidAllowed)
	}
	if !slices.Contains(ignoredKeys(cfg), "tools.web_search_paid_allowed") {
		t.Fatalf("ignored = %v, want tools.web_search_paid_allowed reported", ignoredKeys(cfg))
	}
}

func TestDeepSeekAPIResolvesFromAnOfficialConfiguredProvider(t *testing.T) {
	cfg := Default()
	cfg.Providers = []ProviderEntry{
		{Name: "other", Kind: "openai", BaseURL: "https://api.example.com/v1", Model: "m", resolvedAPIKey: "sk-other"},
		{Name: "lookalike", Kind: "openai", BaseURL: "https://api.deepseek.com.evil.example", Model: "m", resolvedAPIKey: "sk-evil"},
		{Name: "ds", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-flash", resolvedAPIKey: "sk-live"},
	}
	base, key, err := cfg.ResolveDeepSeekAPI()
	if err != nil || base != "https://api.deepseek.com" || key() != "sk-live" {
		t.Fatalf("base=%q err=%v", base, err)
	}
	bare := Default()
	bare.Providers = []ProviderEntry{{Name: "ds", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-flash", APIKeyEnv: "NOT_SET_ANYWHERE_FOR_TEST"}}
	if _, _, err := bare.ResolveDeepSeekAPI(); !errors.Is(err, ErrDeepSeekAPIUnavailable) {
		t.Fatalf("err = %v, want ErrDeepSeekAPIUnavailable for a provider with no key", err)
	}
	none := Default()
	none.Providers = nil
	if _, _, err := none.ResolveDeepSeekAPI(); !errors.Is(err, ErrDeepSeekAPIUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestUserFileChoosesWebFetchBackendsAndTheirAllowance(t *testing.T) {
	d := Default().Tools
	if len(d.WebFetchBackends) != 0 || len(d.WebFetchPaidAllowed) != 0 {
		t.Fatalf("a fetch backend is chosen by default: %+v", d)
	}
	cfg, _ := loadScoped(t, "[tools]\nweb_fetch_backends = [\"local\", \"deepseek_api\"]\nweb_fetch_paid_allowed = [\"deepseek_api\"]\n", "")
	if !slices.Equal(cfg.Tools.WebFetchBackends, []string{"local", "deepseek_api"}) || !slices.Equal(cfg.Tools.WebFetchPaidAllowed, []string{"deepseek_api"}) {
		t.Fatalf("user file's fetch settings were not honoured: %+v", cfg.Tools)
	}
	out := RenderTOMLForScope(cfg, RenderScopeUser)
	for _, want := range []string{`web_fetch_backends = ["local", "deepseek_api"]`, `web_fetch_paid_allowed = ["deepseek_api"]`} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered config lost %q:\n%s", want, out)
		}
	}
}

// A cloned repo can neither add a place pages are fetched through nor agree to
// a charge for the user.
func TestProjectCannotChooseWebFetchBackendsOrAgreeToTheirCharge(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[tools]\nweb_fetch_backends = [\"deepseek_api\"]\nweb_fetch_paid_allowed = [\"deepseek_api\"]\n")
	if len(cfg.Tools.WebFetchBackends) != 0 || len(cfg.Tools.WebFetchPaidAllowed) != 0 {
		t.Fatalf("a project file set fetch routing: %+v", cfg.Tools)
	}
	for _, key := range []string{"tools.web_fetch_backends", "tools.web_fetch_paid_allowed"} {
		if !slices.Contains(ignoredKeys(cfg), key) {
			t.Fatalf("ignored = %v, want %s reported", ignoredKeys(cfg), key)
		}
	}
}
