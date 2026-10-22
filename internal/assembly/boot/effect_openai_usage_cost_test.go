package boot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/pricing"
)

// What a DeepSeek-shaped OpenAI-compatible answer costs reaches the frontend the
// way the vendor bills it: cached input at the cache price, the rest at the input
// price, output at the output price. Through the real assembly, from the bytes the
// provider sent to the quote the frontend is handed.
func TestEffectOpenAIUsageSplitReachesTheQuote(t *testing.T) {
	home := isolateConfigHome(t)
	t.Setenv("REASONIX_HOME", home)
	dir := robustTempDir(t)
	t.Chdir(dir)
	t.Setenv("BOOT_USAGE_COST_TEST_KEY", "sk-test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000000,\"completion_tokens\":100000,\"total_tokens\":1100000,\"prompt_cache_hit_tokens\":950000,\"prompt_cache_miss_tokens\":50000}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	writeUserConfig(t, `
default_model = "relay"
[[providers]]
name = "relay"
kind = "openai"
base_url = "`+srv.URL+`"
model = "deepseek-flash"
api_key_env = "BOOT_USAGE_COST_TEST_KEY"
price = { cache_hit = 0.02, input = 1, output = 4, currency = "CNY" }
`)
	var mu sync.Mutex
	var quoted []string
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.Usage && e.CostQuote != nil {
			mu.Lock()
			quoted = append(quoted, e.CostQuote.Original.Amount+" "+e.CostQuote.Original.Currency)
			mu.Unlock()
		}
	})
	ctrl, err := Build(context.Background(), Options{Sink: sink, Home: home, WorkspaceRoot: dir, Model: "relay"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	if err := ctrl.Run(context.Background(), "reply ok"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	// 950,000 x 0.02 + 50,000 x 1 + 100,000 x 4, per million tokens.
	want := pricing.NewAmountFromFloat((950000*0.02 + 50000*1 + 100000*4) / 1e6)
	found := false
	for _, q := range quoted {
		if q == want.String()+" CNY" {
			found = true
		}
	}
	if !found {
		t.Fatalf("quotes seen = %v, want %s CNY", quoted, want.String())
	}
}
