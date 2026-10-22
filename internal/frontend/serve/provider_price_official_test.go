package serve

import (
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
)

func entryWithCard(baseURL string, card *provider.Pricing) *config.ProviderEntry {
	return &config.ProviderEntry{
		Name: "deepseek", Kind: "openai", BaseURL: baseURL,
		Models: []string{"deepseek-flash"}, Prices: map[string]*provider.Pricing{"deepseek-flash": card},
	}
}

func TestModelPricesSayWhetherACardIsOneWeShipped(t *testing.T) {
	const official = "https://api.deepseek.com"
	for _, tc := range []struct {
		name string
		base string
		card *provider.Pricing
		want bool
	}{
		{"current card", official, &provider.Pricing{CacheHit: 0.003, Input: 0.15, Output: 0.6, Currency: "USD"}, true},
		{"peak card", official, &provider.Pricing{CacheHit: 0.006, Input: 0.3, Output: 1.2, Currency: "USD"}, true},
		{"cached price missing", official, &provider.Pricing{CacheHit: 0, Input: 0.3, Output: 1.2, Currency: "USD"}, false},
		{"a relay has no table", "https://relay.example/v1", &provider.Pricing{CacheHit: 0.003, Input: 0.15, Output: 0.6, Currency: "USD"}, false},
	} {
		got := modelPricesOf(entryWithCard(tc.base, tc.card))["deepseek-flash"]
		if got.Official != tc.want {
			t.Errorf("%s: official = %v, want %v", tc.name, got.Official, tc.want)
		}
	}
}

func TestOfficialPricingMarksOnlyAVendorsOwnEndpoint(t *testing.T) {
	if !officialPricingOf(entryWithCard("https://api.deepseek.com", &provider.Pricing{})) {
		t.Error("DeepSeek's own endpoint was not marked")
	}
	if officialPricingOf(entryWithCard("https://relay.example/v1", &provider.Pricing{})) {
		t.Error("a relay was marked as having an official table")
	}
}
