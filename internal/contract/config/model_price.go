// model_price.go — a price the user declares for one model on a connection, per
// million tokens. It is stored as the model's entry in the provider's prices
// table, which every cost quote reads before the connection-wide fallback.
package config

import (
	"errors"
	"math"
	"net/url"
	"strings"
	"time"
	"unicode"

	"reasonix/internal/contract/pricing"
	"reasonix/internal/contract/provider"
)

// ErrModelPriceInvalid refuses a rate that is negative, not a number, or in a
// currency the price book does not know.
var ErrModelPriceInvalid = errors.New("a model's price must be finite, not negative, and in a known currency")

// ErrPriceSourceInvalid refuses a source that is not a plain http(s) link; the two
// price-source errors are distinct so a caller can say which field is wrong.
var ErrPriceSourceInvalid = errors.New("a price's source must be an http or https link without credentials or control characters")

// ErrPriceCheckedInvalid refuses a check date that is not a calendar date or lies
// in the future.
var ErrPriceCheckedInvalid = errors.New("a price's check date must be a date, YYYY-MM-DD, not later than today in UTC")

// SetModelPrice records price as model's own, or removes it with nil so the
// model falls back to the connection-wide price (and, for a vendor's published
// models, to the shipped rate). Zero is a price: a free model is not an unknown one.
func (e *ProviderEntry) SetModelPrice(model string, price *provider.Pricing) error {
	model = strings.TrimSpace(model)
	if e == nil || model == "" {
		return nil
	}
	if price == nil {
		delete(e.Prices, model)
		if len(e.Prices) == 0 {
			e.Prices = nil
		}
		return nil
	}
	for _, v := range []float64{price.Input, price.Output, price.CacheHit} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return ErrModelPriceInvalid
		}
	}
	currency := pricing.NormalizeCurrency(price.Currency)
	if !isCurrencyCode(currency) {
		return ErrModelPriceInvalid
	}
	if e.Prices == nil {
		e.Prices = map[string]*provider.Pricing{}
	}
	source, checked := strings.TrimSpace(price.SourceURL), strings.TrimSpace(price.Checked)
	if source != "" && !validPriceSource(source) {
		return ErrPriceSourceInvalid
	}
	if checked != "" && !validCheckedDate(checked) {
		return ErrPriceCheckedInvalid
	}
	e.Prices[model] = &provider.Pricing{Input: price.Input, Output: price.Output, CacheHit: price.CacheHit, Currency: currency, SourceURL: source, Checked: checked}
	return nil
}

const maxPriceSourceLen = 500

// utcToday is the date a check date is compared with, in UTC so the answer does not
// depend on where the person is.
var utcToday = func() time.Time { return time.Now().UTC() }

// validPriceSource accepts a link to read, never one to follow on its own: http or
// https, a host, no credentials, no control, format or blank characters, a bounded
// length. A format character is invisible or reorders text, so the page would show a
// link other than the one stored.
func validPriceSource(raw string) bool {
	if len(raw) > maxPriceSourceLen {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.User != nil {
		return false
	}
	return true
}

// validCheckedDate is a calendar date, YYYY-MM-DD, no later than today in UTC.
func validCheckedDate(s string) bool {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	today := utcToday()
	return !d.After(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC))
}

// isCurrencyCode accepts only an ISO-style code, so a price states its unit
// instead of leaving a rate to be read in whatever currency is displayed.
func isCurrencyCode(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
