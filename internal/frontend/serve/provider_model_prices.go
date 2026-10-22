// provider_model_prices.go — the price per million tokens a connection declares
// for each of its models, so a model the price book does not know can be quoted.
package serve

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/pricing"
	"reasonix/internal/contract/provider"
)

// modelPriceView is one model's own price. A model with none stays out of the
// map: an absent entry is "unknown", which is not the same answer as zero.
type modelPriceView struct {
	Input    float64 `json:"input"`
	Output   float64 `json:"output"`
	CacheHit float64 `json:"cacheHit"`
	Currency string  `json:"currency,omitempty"`
	// SourceURL and Checked say where the price was read and when (a UTC date);
	// they describe the price and are not part of it.
	SourceURL string `json:"sourceUrl,omitempty"`
	Checked   string `json:"checked,omitempty"`
	// Official is true when the card is a rate this project shipped for the
	// model; false for the user's own and for anything on an endpoint with no
	// table to measure it against.
	Official bool `json:"official,omitempty"`
}

// officialPricingOf reports whether the entry talks to a vendor's own endpoint,
// the only place a stored card can be compared with that vendor's table.
func officialPricingOf(p *config.ProviderEntry) bool {
	return p != nil && pricing.OfficialProviderForEndpoint(p.BaseURL) != ""
}

func modelPricesOf(p *config.ProviderEntry) map[string]modelPriceView {
	out := map[string]modelPriceView{}
	vendor := ""
	if p != nil {
		vendor = pricing.OfficialProviderForEndpoint(p.BaseURL)
	}
	for _, model := range p.ChatModelList() {
		if price := p.Prices[model]; price != nil {
			card := pricing.RateCard{CacheHit: price.CacheHit, Input: price.Input, Output: price.Output, Currency: price.Currency}
			out[model] = modelPriceView{
				Input: price.Input, Output: price.Output, CacheHit: price.CacheHit, Currency: pricing.NormalizeCurrency(price.Currency),
				SourceURL: price.SourceURL, Checked: price.Checked,
				Official: vendor != "" && pricing.IsOfficialCard(vendor, model, card),
			}
		}
	}
	return orNilMap(out)
}

// applyModelPrices makes the form's answer the whole answer for the models it
// lists: one sent with a price declares it, one sent null or left out has none of
// its own again. Models the form does not list keep what they have.
func applyModelPrices(entry *config.ProviderEntry, models []string, sent *map[string]*modelPriceView) *editRefusal {
	if sent == nil {
		return nil
	}
	for model := range *sent {
		if !slices.Contains(models, strings.TrimSpace(model)) {
			return &editRefusal{"provider.model_prices_unlisted",
				fmt.Sprintf("%q is not one of the selected models", model), map[string]any{"model": model}}
		}
	}
	for _, model := range models {
		var want *modelPriceView
		for k, v := range *sent {
			if strings.TrimSpace(k) == model {
				want = v
			}
		}
		var price *provider.Pricing
		if want != nil {
			price = &provider.Pricing{Input: want.Input, Output: want.Output, CacheHit: want.CacheHit, Currency: want.Currency, SourceURL: want.SourceURL, Checked: want.Checked}
		}
		switch err := entry.SetModelPrice(model, price); {
		case errors.Is(err, config.ErrModelPriceInvalid):
			return &editRefusal{"provider.bad_model_price",
				fmt.Sprintf("the price for %s must be a non-negative number in a known currency", model), map[string]any{"model": model}}
		case errors.Is(err, config.ErrPriceSourceInvalid):
			return &editRefusal{"provider.bad_price_source",
				fmt.Sprintf("the source of the price for %s must be an http or https link", model), map[string]any{"model": model}}
		case errors.Is(err, config.ErrPriceCheckedInvalid):
			return &editRefusal{"provider.bad_price_checked",
				fmt.Sprintf("the check date of the price for %s must be a date, not later than today in UTC", model), map[string]any{"model": model}}
		}
	}
	return nil
}

// applyModelDeclarations applies what the form declares per model beyond the
// connection's own values: limits, then prices, each with the whole-answer rule
// for the models it lists.
func applyModelDeclarations(entry *config.ProviderEntry, models []string, limits *map[string]modelLimitsView, prices *map[string]*modelPriceView) *editRefusal {
	if bad := applyModelLimits(entry, models, limits); bad != nil {
		return bad
	}
	return applyModelPrices(entry, models, prices)
}
