package control

import (
	"os"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
)

func TestDisplayCurrencySettingsNameTheModelsThatCannotBeShownInThePinnedCurrency(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(config.UserConfigPath(), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const provider = `
[[providers]]
name = "deepseek"
kind = "openai"
base_url = "https://api.deepseek.com"
models = ["deepseek-flash"]
[providers.prices.deepseek-flash]
cache_hit = 0
input = 0.3
output = 1.2
currency = "USD"
`
	c := New(Options{})
	t.Cleanup(c.Close)

	write("[billing]\ndisplay_currency = \"CNY\"\n" + provider)
	got := c.DisplayCurrencySettings()
	if len(got.Unconverted) != 1 || got.Unconverted[0].Provider != "deepseek" || got.Unconverted[0].Model != "deepseek-flash" || got.Unconverted[0].Currency != "USD" {
		t.Fatalf("Unconverted = %+v, want deepseek/deepseek-flash in USD", got.Unconverted)
	}

	write("[billing]\ndisplay_currency = \"USD\"\n" + provider)
	if got := c.DisplayCurrencySettings(); len(got.Unconverted) != 0 {
		t.Fatalf("a card already in the pinned currency was listed: %+v", got.Unconverted)
	}
	write(provider)
	if got := c.DisplayCurrencySettings(); got.Unconverted == nil || len(got.Unconverted) != 0 {
		t.Fatalf("with no pinned currency Unconverted must be an empty list, got %#v", got.Unconverted)
	}
}
