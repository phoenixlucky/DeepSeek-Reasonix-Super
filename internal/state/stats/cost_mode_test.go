package stats

import (
	"context"
	"reflect"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/pricing"
	"reasonix/internal/state/usagecatalog"
)

func modeRecords() (string, []map[string]any) {
	return "2026-08-10", []map[string]any{
		{"ts": "2026-08-10T01:00:00Z", "model": "relay/a", "source": "cli", "total": 100, "cost_amount": "1.50", "cost_currency": "CNY", "billing_mode": "payg"},
		{"ts": "2026-08-10T02:00:00Z", "model": "relay/a", "source": "cli", "total": 100, "cost_amount": "2.50", "cost_currency": "CNY"},
		{"ts": "2026-08-10T03:00:00Z", "model": "plan/b", "source": "cli", "total": 400, "cost_amount": "10", "cost_currency": "CNY", "billing_mode": "subscription_equivalent"},
	}
}

func money(m []pricing.Money) map[string]string {
	out := map[string]string{}
	for _, e := range m {
		out[e.Currency] = e.Amount
	}
	return out
}

// A plan's API-priced estimate is not money paid: it never shares a total with
// billed cost, even in the same currency.
func TestSubscriptionEstimateNeverJoinsTheBilledTotal(t *testing.T) {
	dir := testenv.TempDir(t)
	day, lines := modeRecords()
	writeDay(t, dir, day, lines...)
	got := queryAll(t, dir)
	if !reflect.DeepEqual(money(got.Cost), map[string]string{"CNY": "4"}) {
		t.Fatalf("billed total = %+v, want 4 CNY and nothing of the plan's estimate", got.Cost)
	}
	if !reflect.DeepEqual(money(got.EstimateCost), map[string]string{"CNY": "10"}) {
		t.Fatalf("estimate = %+v, want 10 CNY on its own", got.EstimateCost)
	}
	var d DailyTokens
	for _, row := range got.Daily {
		if row.Day == day {
			d = row
		}
	}
	if !reflect.DeepEqual(money(d.Cost), map[string]string{"CNY": "4"}) || !reflect.DeepEqual(money(d.EstimateCost), map[string]string{"CNY": "10"}) {
		t.Fatalf("day cost %+v / estimate %+v", d.Cost, d.EstimateCost)
	}
}

// A day that only has an estimate reports no billed cost, not a zero one.
func TestAnEstimateOnlyDayHasNoBilledCost(t *testing.T) {
	dir := testenv.TempDir(t)
	writeDay(t, dir, "2026-08-11",
		map[string]any{"ts": "2026-08-11T01:00:00Z", "model": "plan/b", "source": "cli", "total": 50, "cost_amount": "3", "cost_currency": "CNY", "billing_mode": "subscription_equivalent"})
	got := queryAll(t, dir)
	if len(got.Cost) != 0 || len(got.EstimateCost) != 1 {
		t.Fatalf("cost %+v estimate %+v", got.Cost, got.EstimateCost)
	}
}

// The rollup catalog and the JSONL read answer the same question the same way,
// so a rebuild that has not finished, or one that failed, changes nothing the
// page shows.
func TestTheCatalogAndTheJSONLReadAgreeOnTheSplit(t *testing.T) {
	dir := testenv.TempDir(t)
	day, lines := modeRecords()
	writeDay(t, dir, day, lines...)
	from, _ := time.Parse(dayLayout, "2026-08-01")
	to, _ := time.Parse(dayLayout, "2026-08-31")
	for _, currency := range []string{"", "CNY", "USD"} {
		filter := SourceFilter{From: from, To: to, Currency: currency}
		fromFiles, err := NewWriter(dir).Query(filter)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		catalog, err := usagecatalog.Open(ctx, testenv.TempDir(t)+"/usage.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if err := catalog.ReconcileDir(ctx, dir); err != nil {
			t.Fatal(err)
		}
		manager := &usageManager{}
		manager.catalog.Store(catalog)
		w := NewWriter(dir)
		w.usage = manager
		fromCatalog, err := w.Query(filter)
		if err != nil {
			t.Fatal(err)
		}
		_ = catalog.Close(ctx)
		if !reflect.DeepEqual(fromFiles, fromCatalog) {
			t.Fatalf("currency %q: the two reads differ\nfiles:   %+v\ncatalog: %+v", currency, fromFiles, fromCatalog)
		}
		if len(fromCatalog.EstimateCost) == 0 {
			t.Fatalf("currency %q: the catalog read lost the estimate", currency)
		}
	}
}

func queryThrough(t *testing.T, dir string, catalog *usagecatalog.Catalog, filter SourceFilter) RangeStats {
	t.Helper()
	manager := &usageManager{}
	manager.catalog.Store(catalog)
	w := NewWriter(dir)
	w.usage = manager
	got, err := w.Query(filter)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// While the projection has not been built, or cannot be read, the page is
// answered from the stats files: slower, never empty and never different.
func TestAnUnbuiltOrUnreadableCatalogFallsBackToTheFilesWithTheSameAnswer(t *testing.T) {
	dir := testenv.TempDir(t)
	day, lines := modeRecords()
	writeDay(t, dir, day, lines...)
	from, _ := time.Parse(dayLayout, "2026-08-01")
	to, _ := time.Parse(dayLayout, "2026-08-31")
	filter := SourceFilter{From: from, To: to}
	want, err := NewWriter(dir).Query(filter)
	if err != nil || len(want.EstimateCost) == 0 {
		t.Fatalf("the files answer %+v (%v)", want, err)
	}
	ctx := context.Background()

	unbuilt, err := usagecatalog.Open(ctx, testenv.TempDir(t)+"/unbuilt.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer unbuilt.Close(ctx)
	if got := queryThrough(t, dir, unbuilt, filter); !reflect.DeepEqual(got, want) {
		t.Fatalf("an unbuilt catalog changed the answer\nwant %+v\ngot  %+v", want, got)
	}

	broken, err := usagecatalog.Open(ctx, testenv.TempDir(t)+"/broken.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := broken.ReconcileDir(ctx, dir); err != nil {
		t.Fatal(err)
	}
	_ = broken.Close(ctx)
	if got := queryThrough(t, dir, broken, filter); !reflect.DeepEqual(got, want) {
		t.Fatalf("an unreadable catalog changed the answer\nwant %+v\ngot  %+v", want, got)
	}
}
