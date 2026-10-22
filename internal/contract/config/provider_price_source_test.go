package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

const pricedProvider = `config_version = 12
[[providers]]
name = "relay"
kind = "anthropic"
base_url = "https://relay.example/anthropic"
model = "m"
models = ["m"]
prices = { "m" = { cache_hit = 0.1, input = 1, output = 2, currency = "USD", source_url = "https://relay.example/pricing", checked = "2026-10-01" } }
billing_mode = "subscription_equivalent"
`

func relayEntry(path string) ProviderEntry {
	for _, p := range LoadForEdit(path).Providers {
		if p.Name == "relay" {
			return p
		}
	}
	return ProviderEntry{}
}

// rewriteAs1x leaves the file as the 1.x line does after one of its saves: the
// keys it has never heard of are not there.
func rewriteAs1x(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := regexp.MustCompile(`(?m)^billing_mode\s*=.*\n`).ReplaceAllString(string(data), "")
	text = regexp.MustCompile(`, (source_url|checked) = "[^"]*"`).ReplaceAllString(text, "")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAPriceKeepsItsSourceAndCheckDateThroughTheFile(t *testing.T) {
	path := writeUserConfigFile(t, pricedProvider)
	p := relayEntry(path).Prices["m"]
	if p == nil || p.SourceURL != "https://relay.example/pricing" || p.Checked != "2026-10-01" {
		t.Fatalf("price after load = %+v", p)
	}
	if err := LoadForEdit(path).SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if got := relayEntry(path).Prices["m"]; got == nil || got.SourceURL != p.SourceURL || got.Checked != p.Checked {
		t.Fatalf("price after a save = %+v", got)
	}
}

func TestSourceAndBillingModeDroppedByARewriteComeBackAtStart(t *testing.T) {
	path := writeUserConfigFile(t, pricedProvider)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	rewriteAs1x(t, path)
	if p := relayEntry(path); p.BillingMode == "subscription_equivalent" || p.Prices["m"].SourceURL != "" {
		t.Fatalf("setup: the rewrite kept %q / %+v", p.BillingMode, p.Prices["m"])
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	p := relayEntry(path)
	if p.BillingMode != "subscription_equivalent" || p.Prices["m"].SourceURL != "https://relay.example/pricing" || p.Prices["m"].Checked != "2026-10-01" {
		t.Fatalf("after a start: mode %q, price %+v", p.BillingMode, p.Prices["m"])
	}
}

func TestAClearedSourceOrModeIsNotRestored(t *testing.T) {
	path := writeUserConfigFile(t, pricedProvider)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	e := &cfg.Providers[0]
	e.BillingMode = "payg"
	e.Prices["m"].SourceURL, e.Prices["m"].Checked = "", ""
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if err := NoteProviderKeys(path, e); err != nil {
		t.Fatal(err)
	}
	rewriteAs1x(t, path)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if p := relayEntry(path); p.BillingMode == "subscription_equivalent" || p.Prices["m"].SourceURL != "" || p.Prices["m"].Checked != "" {
		t.Fatalf("a cleared key came back: mode %q, price %+v", p.BillingMode, p.Prices["m"])
	}
}

func TestAChangedPriceDoesNotInheritTheOldSource(t *testing.T) {
	path := writeUserConfigFile(t, pricedProvider)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	cfg.Providers[0].Prices["m"].Input = 3
	cfg.Providers[0].Prices["m"].SourceURL, cfg.Providers[0].Prices["m"].Checked = "", ""
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if p := relayEntry(path).Prices["m"]; p.Input != 3 || p.SourceURL != "" || p.Checked != "" {
		t.Fatalf("a repriced model took the old source: %+v", p)
	}
}

// A record written when only the balance format was kept is still honoured.
func TestTheBalanceFormatsFileIsStillRead(t *testing.T) {
	path := writeUserConfigFile(t, relayAndVendorProviders)
	dir := filepath.Dir(path)
	legacy := `{"entries":{"relay":{"url":"https://relay.example/user/balance","format":"moonshot"}}}`
	if err := os.WriteFile(filepath.Join(dir, "balance-formats.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if got := balanceFormats(path)["relay"]; got != "moonshot" {
		t.Fatalf("the legacy record was not honoured: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "provider-keys.json")); err != nil {
		t.Fatalf("the general record was not written: %v", err)
	}
}

func TestSetModelPriceChecksTheSourceAndTheDate(t *testing.T) {
	utcToday = func() time.Time { return time.Date(2026, 10, 18, 23, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { utcToday = func() time.Time { return time.Now().UTC() } })
	long := "https://example.com/" + strings.Repeat("a", 600)
	cases := []struct {
		name    string
		url     string
		checked string
		want    error
	}{
		{"https", "https://example.com/p", "2026-10-18", nil},
		{"none", "", "", nil},
		{"date only", "", "2026-10-17", nil},
		{"ftp", "ftp://example.com/p", "", ErrPriceSourceInvalid},
		{"javascript", "javascript:alert(1)", "", ErrPriceSourceInvalid},
		{"userinfo", "https://user:pw@example.com/p", "", ErrPriceSourceInvalid},
		{"no host", "https:///p", "", ErrPriceSourceInvalid},
		{"newline", "https://example.com/p\nHost: x", "", ErrPriceSourceInvalid},
		{"nul", "https://example.com/p\x00", "", ErrPriceSourceInvalid},
		{"space", "https://example.com/a b", "", ErrPriceSourceInvalid},
		{"zero-width space in the host", "https://api-docs.deep\u200bseek.com/pricing", "", ErrPriceSourceInvalid},
		{"soft hyphen in the host", "https://api-docs.deep\u00adseek.com/pricing", "", ErrPriceSourceInvalid},
		{"bidi override in the path", "https://gw.example/\u202elmth.gnicirp", "", ErrPriceSourceInvalid},
		{"too long", long, "", ErrPriceSourceInvalid},
		{"bad date", "", "18/10/2026", ErrPriceCheckedInvalid},
		{"impossible date", "", "2026-02-30", ErrPriceCheckedInvalid},
		{"tomorrow in UTC", "", "2026-10-19", ErrPriceCheckedInvalid},
	}
	for _, c := range cases {
		e := &ProviderEntry{Name: "p"}
		err := e.SetModelPrice("m", &provider.Pricing{Input: 1, Output: 2, CacheHit: 1, Currency: "USD", SourceURL: c.url, Checked: c.checked})
		if !errors.Is(err, c.want) && !(c.want == nil && err == nil) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
		if c.want == nil && c.url != "" && e.Prices["m"].SourceURL != c.url {
			t.Errorf("%s: the accepted source was not stored", c.name)
		}
	}
}
