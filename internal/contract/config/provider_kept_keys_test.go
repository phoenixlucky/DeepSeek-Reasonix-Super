package config

import "testing"

const relayAndVendorProviders = `config_version = 12
[[providers]]
name = "relay"
kind = "anthropic"
base_url = "https://relay.example/anthropic"
model = "m"
balance_url = "https://relay.example/user/balance"

[[providers]]
name = "official"
kind = "anthropic"
base_url = "https://api.deepseek.com/anthropic"
model = "deepseek-flash"
balance_url = "https://api.deepseek.com/user/balance"

[[providers]]
name = "nowallet"
kind = "anthropic"
base_url = "https://other.example/anthropic"
model = "m"
`

func balanceFormats(path string) map[string]string {
	out := map[string]string{}
	for _, p := range LoadForEdit(path).Providers {
		out[p.Name] = p.BalanceFormat
	}
	return out
}

// An address that is not a vendor's own host was always read as DeepSeek's
// shape; the upgrade says so out loud for the entries that have one, and leaves
// a vendor's own host (implied) and an entry with no wallet alone.
func TestBalanceFormatUpgradeDeclaresDeepSeekOnlyWhereTheHostDoesNotImplyIt(t *testing.T) {
	path := writeUserConfigFile(t, relayAndVendorProviders)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	got := balanceFormats(path)
	if got["relay"] != "deepseek" || got["official"] != "" || got["nowallet"] != "" {
		t.Fatalf("balance_format after upgrade = %v, want deepseek only on the relay", got)
	}
}

// The 1.x line rewrites the shared file from the keys it knows, which drops a
// format it has never heard of; the next start puts it back, once the address is
// still the one it was declared for.
func TestAFormatDroppedByARewriteIsPutBackAtTheNextStart(t *testing.T) {
	path := writeUserConfigFile(t, relayAndVendorProviders)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "relay" {
			cfg.Providers[i].BalanceFormat = "moonshot"
		}
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if got := balanceFormats(path)["relay"]; got != "moonshot" {
		t.Fatalf("a chosen format was rewritten: %q", got)
	}

	dropped := LoadForEdit(path)
	for i := range dropped.Providers {
		dropped.Providers[i].BalanceFormat = ""
	}
	if err := dropped.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if got := balanceFormats(path)["relay"]; got != "" {
		t.Fatalf("setup: the rewrite kept the format (%q)", got)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if got := balanceFormats(path)["relay"]; got != "moonshot" {
		t.Fatalf("after the rewrite and a start the format is %q, want moonshot back", got)
	}
}

// A format the person removed, or an address they changed, is not given back.
func TestARemovedFormatOrAChangedAddressIsNotRestored(t *testing.T) {
	path := writeUserConfigFile(t, relayAndVendorProviders)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "relay" {
			cfg.Providers[i].BalanceFormat = ""
			if err := NoteProviderKeys(path, &cfg.Providers[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if got := balanceFormats(path)["relay"]; got != "" {
		t.Fatalf("a removed format came back: %q", got)
	}

	path = writeUserConfigFile(t, relayAndVendorProviders)
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	moved := LoadForEdit(path)
	for i := range moved.Providers {
		if moved.Providers[i].Name == "relay" {
			moved.Providers[i].BalanceFormat = ""
			moved.Providers[i].BalanceURL = "https://other-relay.example/user/balance"
		}
	}
	if err := moved.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUserConfigUpgradesOnStartup(path); err != nil {
		t.Fatal(err)
	}
	if got := balanceFormats(path)["relay"]; got != "" {
		t.Fatalf("a format was restored onto a different address: %q", got)
	}
}

func TestAnUnknownBalanceFormatIsRefused(t *testing.T) {
	e := ProviderEntry{Name: "p", Kind: "anthropic", BaseURL: "https://x", Model: "m", BalanceFormat: "mystery"}
	if err := validateProvider(e); err == nil {
		t.Fatal("balance_format = mystery was accepted")
	}
	e.BalanceFormat = "moonshot"
	if err := validateProvider(e); err != nil {
		t.Fatalf("a declared format was refused: %v", err)
	}
}
