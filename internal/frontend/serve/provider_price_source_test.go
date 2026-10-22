package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditProviderStoresAPriceSourceAndShowsItBack(t *testing.T) {
	srv := newRichProviderServer(t)
	resp := postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":{"input":3,"output":15,"cacheHit":0.3,"currency":"USD","sourceUrl":"https://relay.example/pricing","checked":"2020-01-02"}}}`)
	b, _ := readAllString(resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /providers/edit = %d %s", resp.StatusCode, b)
	}
	alpha, _ := loadEntry(t, "rich/alpha")
	p := alpha.PriceForModel("alpha")
	if p == nil || p.SourceURL != "https://relay.example/pricing" || p.Checked != "2020-01-02" {
		t.Fatalf("stored price = %+v", p)
	}
	list := getProviders(t, srv)
	for _, v := range list {
		if v.Name == "rich" && (v.ModelPrices["alpha"].SourceURL != "https://relay.example/pricing" || v.ModelPrices["alpha"].Checked != "2020-01-02") {
			t.Fatalf("the list lost the source: %+v", v.ModelPrices["alpha"])
		}
	}
}

func TestEditProviderRefusesABadPriceSourceOrDate(t *testing.T) {
	srv := newRichProviderServer(t)
	for _, tc := range []struct{ field, code string }{
		{`"sourceUrl":"javascript:alert(1)"`, "provider.bad_price_source"},
		{`"sourceUrl":"https://user:pw@example.com/"`, "provider.bad_price_source"},
		{`"sourceUrl":"https://example.com/a\nb"`, "provider.bad_price_source"},
		{`"checked":"yesterday"`, "provider.bad_price_checked"},
		{`"checked":"2999-01-01"`, "provider.bad_price_checked"},
	} {
		resp := postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":{"input":1,"output":2,"cacheHit":1,"currency":"USD",`+tc.field+`}}}`)
		b, _ := readAllString(resp)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(b, tc.code) {
			t.Fatalf("%s: got %d %s, want 400 %s", tc.field, resp.StatusCode, b, tc.code)
		}
	}
}

func TestEditProviderSetsAndReportsTheBillingMode(t *testing.T) {
	srv := newRichProviderServer(t)
	if status, body := editBalance(t, srv, `"billingMode":"subscription_equivalent"`); status != http.StatusNoContent {
		t.Fatalf("edit = %d %s", status, body)
	}
	if got := storedRich(t).ProviderBillingMode(); got != "subscription_equivalent" {
		t.Fatalf("stored mode = %q", got)
	}
	for _, v := range getProviders(t, srv) {
		if v.Name == "rich" && v.BillingMode != "subscription_equivalent" {
			t.Fatalf("the list says %q", v.BillingMode)
		}
	}
	if status, body := editBalance(t, srv, `"billingMode":"free-lunch"`); status != http.StatusBadRequest || !strings.Contains(body, "provider.billing_mode_unknown") {
		t.Fatalf("an unknown mode: %d %s", status, body)
	}
	if status, _ := editBalance(t, srv, `"balanceFormat":"deepseek"`); status != http.StatusNoContent {
		t.Fatal("setup")
	}
	if got := storedRich(t).ProviderBillingMode(); got != "subscription_equivalent" {
		t.Fatalf("an edit that did not send the mode changed it to %q", got)
	}
}

type pricedProviderView struct {
	Name        string                    `json:"name"`
	BillingMode string                    `json:"billingMode"`
	ModelPrices map[string]modelPriceView `json:"modelPrices"`
}

func getProviders(t *testing.T, srv *httptest.Server) []pricedProviderView {
	t.Helper()
	resp, err := http.Get(srv.URL + "/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []pricedProviderView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestASourceOrDateAloneDoesNotRebuildTheConversationButAModeOrRateDoes(t *testing.T) {
	srv := newRichProviderServer(t)
	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":{"input":3,"output":15,"cacheHit":0.3,"currency":"USD"}}}`).Body.Close()
	before := assemblyShape(mustEntry(t, "rich"))
	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":{"input":3,"output":15,"cacheHit":0.3,"currency":"USD","sourceUrl":"https://example.com/p","checked":"2020-01-02"}}}`).Body.Close()
	if after := assemblyShape(mustEntry(t, "rich")); after != before {
		t.Fatalf("noting a source changed the shape:\n%s\n%s", before, after)
	}
	editBalance(t, srv, `"billingMode":"subscription_equivalent"`)
	if after := assemblyShape(mustEntry(t, "rich")); after == before {
		t.Fatal("a billing mode change left the shape alone, so a running conversation would keep quoting the old mode")
	}
}

func TestRemovingAConnectionForgetsTheKeysKeptForIt(t *testing.T) {
	srv := newRichProviderServer(t)
	postProvider(t, srv.URL, "/providers/edit", `{`+priceEditBase+`"modelPrices":{"alpha":{"input":3,"output":15,"cacheHit":0.3,"currency":"USD","sourceUrl":"https://example.com/p","checked":"2020-01-02"}}}`).Body.Close()
	if !strings.Contains(readBalanceLedger(t), "https://example.com/p") {
		t.Fatalf("setup: the source was not recorded: %s", readBalanceLedger(t))
	}
	resp := postProvider(t, srv.URL, "/providers/remove", `{"name":"rich"}`)
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("remove = %d", resp.StatusCode)
	}
	for _, v := range getProviders(t, srv) {
		if v.Name == "rich" {
			t.Fatal("the connection is still listed")
		}
	}
	if strings.Contains(readBalanceLedger(t), "https://example.com/p") {
		t.Fatalf("a removed connection's source is still recorded: %s", readBalanceLedger(t))
	}
}
