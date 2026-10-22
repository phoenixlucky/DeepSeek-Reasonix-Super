package serve

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/pricing"
)

const priceEditBase = `"name":"rich","models":["alpha","beta"],"default":"alpha","vision":[],`

func TestEditProviderStoresAModelsPriceAndClearsIt(t *testing.T) {
	srv := newRichProviderServer(t)
	resp := postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`
		"modelPrices":{"alpha":{"input":3,"output":15,"cacheHit":0.3,"currency":"usd"}}}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /providers/edit = %d", resp.StatusCode)
	}
	alpha, _ := loadEntry(t, "rich/alpha")
	if p := alpha.PriceForModel("alpha"); p == nil || p.Input != 3 || p.Output != 15 || p.CacheHit != 0.3 || pricing.NormalizeCurrency(p.Currency) != "USD" {
		t.Fatalf("alpha is priced %+v, want 3/15/0.3 USD", p)
	}
	if beta, _ := loadEntry(t, "rich/beta"); beta.PriceForModel("beta") != nil {
		t.Fatalf("beta declares nothing but is priced %+v", beta.PriceForModel("beta"))
	}
	if got := listedRich(t, srv.URL).ModelPrices["alpha"]; got.Input != 3 || got.Currency != "USD" {
		t.Fatalf("listed price = %+v", got)
	}

	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":null}}`).Body.Close()
	alpha, _ = loadEntry(t, "rich/alpha")
	if alpha.PriceForModel("alpha") != nil {
		t.Fatalf("clearing left alpha priced %+v", alpha.PriceForModel("alpha"))
	}
	raw, _ := os.ReadFile(config.UserConfigPath())
	if strings.Contains(string(raw), "prices.alpha") {
		t.Fatalf("the cleared price is still on file:\n%s", raw)
	}
}

// Free is a price and unknown is not: a model sent with zeros is quoted at zero,
// and one left unpriced stays unknown.
func TestEditProviderKeepsAFreeModelApartFromAnUnpricedOne(t *testing.T) {
	srv := newRichProviderServer(t)
	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`
		"modelPrices":{"alpha":{"input":0,"output":0,"cacheHit":0,"currency":"CNY"}}}`).Body.Close()
	alpha, _ := loadEntry(t, "rich/alpha")
	beta, _ := loadEntry(t, "rich/beta")
	if p := alpha.PriceForModel("alpha"); p == nil || p.Input != 0 || pricing.NormalizeCurrency(p.Currency) != "CNY" {
		t.Fatalf("alpha = %+v, want a stored zero price", p)
	}
	if beta.PriceForModel("beta") != nil {
		t.Fatal("beta became priced")
	}
}

func TestEditProviderLeavesPricesAloneWhenNotSent(t *testing.T) {
	srv := newRichProviderServer(t)
	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"beta":{"input":1,"output":2,"cacheHit":1,"currency":"CNY"}}}`).Body.Close()
	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelLimits":{}}`).Body.Close()
	if beta, _ := loadEntry(t, "rich/beta"); beta.PriceForModel("beta") == nil {
		t.Fatal("an edit that sent no prices removed beta's")
	}
}

func TestEditProviderRefusesBadModelPrices(t *testing.T) {
	srv := newRichProviderServer(t)
	before, _ := loadEntry(t, "rich/alpha")
	was := before.PriceForModel("alpha")
	for _, tc := range []struct{ body, code string }{
		{`"modelPrices":{"alpha":{"input":-1,"output":2,"cacheHit":0}}`, "provider.bad_model_price"},
		{`"modelPrices":{"alpha":{"input":1,"output":2,"cacheHit":0,"currency":"zorkmids"}}`, "provider.bad_model_price"},
		{`"modelPrices":{"alpha":{"input":1,"output":2,"cacheHit":0}}`, "provider.bad_model_price"},
		{`"modelPrices":{"gamma":{"input":1,"output":2,"cacheHit":0}}`, "provider.model_prices_unlisted"},
	} {
		resp := postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+tc.body+`}`)
		b, _ := readAllString(resp)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(b, tc.code) {
			t.Fatalf("%s: got %d %s, want 400 %s", tc.body, resp.StatusCode, b, tc.code)
		}
		if alpha, _ := loadEntry(t, "rich/alpha"); fmt.Sprint(alpha.PriceForModel("alpha")) != fmt.Sprint(was) {
			t.Fatalf("%s: a refused edit changed alpha's price from %+v to %+v", tc.body, was, alpha.PriceForModel("alpha"))
		}
	}
}

func TestEditProviderPriceChangeReachesTheRunningConversation(t *testing.T) {
	srv := newRichProviderServer(t)
	resp := postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":{"input":3,"output":15,"cacheHit":0.3,"currency":"USD"}}}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /providers/edit = %d", resp.StatusCode)
	}
	if shape := assemblyShape(mustEntry(t, "rich")); !strings.Contains(shape, `[3,15,0.3`) {
		t.Fatalf("the assembly shape ignores prices, so a price edit would not rebuild: %s", shape)
	}
}

func mustEntry(t *testing.T, name string) *config.ProviderEntry {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == name {
			return &cfg.Providers[i]
		}
	}
	t.Fatalf("no provider %s", name)
	return nil
}
