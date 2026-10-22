package config

import (
	"testing"

	"reasonix/internal/contract/provider"
)

func deepSeekWithCard(card *provider.Pricing, display string) *Config {
	c := Default()
	c.Providers = []ProviderEntry{{
		Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com",
		Models: []string{"deepseek-flash"}, Prices: map[string]*provider.Pricing{"deepseek-flash": card},
	}}
	if err := c.SetDisplayCurrency(display); err != nil {
		panic(err)
	}
	return c
}

func TestUnconvertedPricesNamesACardThatCannotBeShownInThePinnedCurrency(t *testing.T) {
	damaged := &provider.Pricing{CacheHit: 0, Input: 0.3, Output: 1.2, Currency: "USD"}
	got := deepSeekWithCard(damaged, "CNY").UnconvertedPrices()
	want := []UnconvertedPrice{{Provider: "deepseek", Model: "deepseek-flash", Currency: "USD"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("UnconvertedPrices = %+v, want %+v", got, want)
	}
}

func TestUnconvertedPricesLeavesWhatCanBeShown(t *testing.T) {
	official := &provider.Pricing{CacheHit: 0.003, Input: 0.15, Output: 0.6, Currency: "USD"}
	peak := &provider.Pricing{CacheHit: 0.006, Input: 0.3, Output: 1.2, Currency: "USD"}
	older := &provider.Pricing{CacheHit: 0.0028, Input: 0.14, Output: 0.28, Currency: "USD"}
	damaged := &provider.Pricing{CacheHit: 0, Input: 0.3, Output: 1.2, Currency: "USD"}
	for _, tc := range []struct {
		name    string
		card    *provider.Pricing
		display string
	}{
		{"current card in another currency has an official peer", official, "CNY"},
		{"peak card has an official peer", peak, "CNY"},
		{"a card we shipped earlier has an official peer", older, "CNY"},
		{"the card already in the pinned currency", damaged, "USD"},
		{"no pinned currency", damaged, ""},
	} {
		if got := deepSeekWithCard(tc.card, tc.display).UnconvertedPrices(); len(got) != 0 {
			t.Errorf("%s: UnconvertedPrices = %+v, want none", tc.name, got)
		}
	}
	relay := deepSeekWithCard(damaged, "CNY")
	relay.Providers[0].BaseURL = "https://relay.example/v1"
	if got := relay.UnconvertedPrices(); len(got) != 0 {
		t.Errorf("a relay has no official table to be measured against, got %+v", got)
	}
}
