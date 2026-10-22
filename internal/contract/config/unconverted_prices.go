package config

import (
	"strings"

	"reasonix/internal/contract/pricing"
)

// UnconvertedPrice names a model of a vendor's own endpoint whose stored price
// cannot be shown in the currency the user pinned: it is in another currency and
// is not a rate this project shipped, so the vendor's table has no peer for it.
type UnconvertedPrice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Currency string `json:"currency"`
}

// UnconvertedPrices lists them while a display currency is pinned. Auto and a
// card already in the pinned currency ask for nothing, and an endpoint that is not
// a vendor's own has no table to measure a card against.
func (c *Config) UnconvertedPrices() []UnconvertedPrice {
	display := c.DisplayCurrencyPref()
	if display == "" {
		return nil
	}
	var out []UnconvertedPrice
	for i := range c.Providers {
		p := &c.Providers[i]
		vendor := officialProviderKind(p)
		if vendor == "" {
			continue
		}
		for _, model := range p.ChatModelList() {
			card := p.PriceForModel(model)
			if card == nil || pricing.NormalizeCurrency(card.Currency) == display {
				continue
			}
			rate := pricing.RateCard{CacheHit: card.CacheHit, Input: card.Input, Output: card.Output, Currency: card.Currency}
			if pricing.IsOfficialCard(vendor, model, rate) {
				continue
			}
			out = append(out, UnconvertedPrice{Provider: p.Name, Model: strings.TrimSpace(model), Currency: pricing.NormalizeCurrency(card.Currency)})
		}
	}
	return out
}
