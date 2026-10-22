package config

import (
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

var mondayTenInBeijing = time.Date(2026, 10, 12, 10, 0, 0, 0, time.FixedZone("", 8*3600))

// A model on the vendor's own endpoint at the vendor's own list price is billed
// under its schedule; the same model on a relay, or at a price the user typed,
// is billed under neither.
func TestLiveTariffIsOnlyForTheVendorsOwnRateOnTheVendorsOwnEndpoint(t *testing.T) {
	c := Default()
	flash, ok := c.Provider("deepseek-flash")
	if !ok {
		t.Fatal("no default deepseek-flash provider")
	}
	ref := flash.Name + "/" + flash.DefaultModel()
	tariff, ok := c.LiveTariff(ref, mondayTenInBeijing)
	if !ok || !tariff.Peak {
		t.Fatalf("official rate at Monday 10:00 Beijing = %+v %v, want peak", tariff, ok)
	}
	if tariff, _ := c.LiveTariff(ref, time.Date(2026, 10, 17, 10, 0, 0, 0, time.FixedZone("", 8*3600))); tariff.Peak {
		t.Fatal("a Saturday is off-peak")
	}

	relay := *flash
	relay.Name = "relay"
	relay.BaseURL = "https://relay.example/v1"
	c.Providers = append(c.Providers, relay)
	if _, ok := c.LiveTariff("relay/"+flash.DefaultModel(), mondayTenInBeijing); ok {
		t.Fatal("a relay was given the vendor's schedule")
	}

	custom := *flash
	custom.Name = "custom"
	price := *flash.PriceForModel(flash.DefaultModel())
	price.Input = price.Input * 3
	custom.Price = &provider.Pricing{CacheHit: price.CacheHit, Input: price.Input, Output: price.Output, Currency: price.Currency}
	c.Providers = append(c.Providers, custom)
	if _, ok := c.LiveTariff("custom/"+flash.DefaultModel(), mondayTenInBeijing); ok {
		t.Fatal("a price the user typed was given the vendor's schedule")
	}
}
