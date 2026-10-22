package config

import (
	"errors"
	"math"
	"testing"

	"reasonix/internal/contract/provider"
)

func TestSetModelPriceStoresNormalizesAndClears(t *testing.T) {
	e := &ProviderEntry{Name: "x", Models: []string{"m"}}
	if err := e.SetModelPrice(" m ", &provider.Pricing{Input: 1, Output: 2, CacheHit: 0.5, Currency: "rmb"}); err != nil {
		t.Fatal(err)
	}
	if p := e.PriceForModel("m"); p == nil || p.Currency != "CNY" || p.Input != 1 {
		t.Fatalf("price = %+v", p)
	}
	if err := e.SetModelPrice("m", nil); err != nil {
		t.Fatal(err)
	}
	if e.PriceForModel("m") != nil || e.Prices != nil {
		t.Fatalf("cleared price left %+v", e.Prices)
	}
}

func TestSetModelPriceKeepsZeroAsAPriceAndRefusesTheUnusable(t *testing.T) {
	e := &ProviderEntry{Name: "x"}
	if err := e.SetModelPrice("free", &provider.Pricing{Currency: "USD"}); err != nil || e.PriceForModel("free") == nil {
		t.Fatalf("a zero price was not stored: %v %+v", err, e.Prices)
	}
	for name, p := range map[string]*provider.Pricing{
		"negative": {Input: -1},
		"nan":      {Output: math.NaN()},
		"inf":      {CacheHit: math.Inf(1)},
		"currency": {Input: 1, Currency: "zorkmids"},
		"blank":    {Input: 1},
	} {
		if err := e.SetModelPrice("bad", p); !errors.Is(err, ErrModelPriceInvalid) {
			t.Errorf("%s: err = %v, want ErrModelPriceInvalid", name, err)
		}
	}
	if e.PriceForModel("bad") != nil {
		t.Fatal("a refused price was stored")
	}
}

func TestAModelsOwnPriceBeatsTheConnectionWideOne(t *testing.T) {
	e := &ProviderEntry{Name: "x", Price: &provider.Pricing{Input: 9, Output: 9, Currency: "USD"}}
	if got := e.PriceForModel("other"); got == nil || got.Input != 9 {
		t.Fatalf("an unpriced model should fall back to the connection price, got %+v", got)
	}
	_ = e.SetModelPrice("m", &provider.Pricing{Input: 1, Output: 2, Currency: "USD"})
	if got := e.PriceForModel("m"); got.Input != 1 {
		t.Fatalf("own price lost to the fallback: %+v", got)
	}
}
