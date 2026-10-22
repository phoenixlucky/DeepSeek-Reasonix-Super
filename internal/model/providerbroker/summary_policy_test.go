package providerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

func TestSummaryCatalogRequiresFreshPolicySupport(t *testing.T) {
	for _, mode := range []string{"old", "false", "invalid", "server error", "unauthorized", "timeout", "supported"} {
		t.Run(mode, func(t *testing.T) {
			var catalog, posts atomic.Int32
			var behavior atomic.Value
			behavior.Store("supported")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == PathCatalog {
					catalog.Add(1)
					switch behavior.Load().(string) {
					case "old":
						_, _ = w.Write([]byte(`{"providers":[]}`))
					case "false":
						_, _ = w.Write([]byte(`{"providers":[],"summary_tool_choice_none":false}`))
					case "invalid":
						_, _ = w.Write([]byte(`not json`))
					case "server error":
						w.WriteHeader(http.StatusInternalServerError)
					case "unauthorized":
						w.WriteHeader(http.StatusUnauthorized)
					case "timeout":
						<-r.Context().Done()
					default:
						_, _ = w.Write([]byte(`{"providers":[],"summary_tool_choice_none":true}`))
					}
					return
				}
				posts.Add(1)
				_ = json.NewEncoder(w).Encode(encodeChunk(provider.Chunk{Type: provider.ChunkDone}))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, testToken, srv.Client())
			p, err := c.Resolve(provider.Selection{Ref: "p/m"})
			if err != nil {
				t.Fatal(err)
			}
			behavior.Store(mode)
			before := catalog.Load()
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			ch, err := p.Stream(ctx, provider.Request{Summary: true, Tools: []provider.ToolSchema{{Name: "read_file"}}})
			if mode == "supported" {
				if err != nil {
					t.Fatal(err)
				}
				drain(t, ch)
				if posts.Load() != 1 {
					t.Fatal("supported summary did not run")
				}
			} else if (mode != "timeout" && !errors.Is(err, provider.ErrSummaryToolsUnsupported)) || (mode == "timeout" && !errors.Is(err, context.DeadlineExceeded)) || posts.Load() != 0 {
				t.Fatalf("unsupported summary sent: err=%v posts=%d", err, posts.Load())
			}
			if catalog.Load() != before+1 {
				t.Fatal("summary relied on cached catalog capability")
			}
			behavior.Store("old")
			before = catalog.Load()
			ch, err = p.Stream(t.Context(), provider.Request{Summary: true})
			if err != nil {
				t.Fatal(err)
			}
			drain(t, ch)
			ch, err = p.Stream(t.Context(), provider.Request{Tools: []provider.ToolSchema{{Name: "read_file"}}})
			if err != nil {
				t.Fatal(err)
			}
			drain(t, ch)
			if catalog.Load() != before {
				t.Fatal("ordinary and independent requests gained a capability dependency")
			}
		})
	}
}

func TestSummaryCapabilityIsPinnedToTheAuthenticatedTarget(t *testing.T) {
	var aPosts, bPosts atomic.Int32
	var target atomic.Value
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderToken) != "token-b" {
			t.Error("wrong B token")
		}
		if r.URL.Path == PathCatalog {
			_, _ = w.Write([]byte(`{"providers":[]}`))
			return
		}
		bPosts.Add(1)
	}))
	defer b.Close()
	var switchDuringProbe atomic.Bool
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderToken) != "token-a" {
			t.Error("wrong A token")
		}
		if r.URL.Path == PathCatalog {
			if switchDuringProbe.Load() {
				target.Store(b.URL)
			}
			_, _ = w.Write([]byte(`{"providers":[],"summary_tool_choice_none":true}`))
			return
		}
		aPosts.Add(1)
		_ = json.NewEncoder(w).Encode(encodeChunk(provider.Chunk{Type: provider.ChunkDone}))
	}))
	defer a.Close()
	target.Store(a.URL)
	c := NewEndpointClient(func() (string, string, error) {
		url := target.Load().(string)
		token := "token-a"
		if url == b.URL {
			token = "token-b"
		}
		return url, token, nil
	}, a.Client())
	p, err := c.Resolve(provider.Selection{Ref: "p/m"})
	if err != nil {
		t.Fatal(err)
	}
	req := provider.Request{Summary: true, Tools: []provider.ToolSchema{{Name: "read_file"}}}
	switchDuringProbe.Store(true)
	ch, err := p.Stream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	drain(t, ch)
	if aPosts.Load() != 1 || bPosts.Load() != 0 {
		t.Fatalf("probe and POST did not share target: A=%d B=%d", aPosts.Load(), bPosts.Load())
	}
	if _, err = p.Stream(t.Context(), req); err == nil || bPosts.Load() != 0 {
		t.Fatal("positive A capability leaked to legacy B")
	}
}

func TestCurrentServerAdvertisesSummaryPolicy(t *testing.T) {
	p := &fakeProvider{name: "p"}
	s, err := NewServer(staticResolver(p), testToken)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, PathCatalog, nil)
	r.Header.Set(HeaderToken, testToken)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var body struct {
		Summary bool `json:"summary_tool_choice_none"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || !body.Summary {
		t.Fatal("new broker does not advertise policy")
	}
}

func TestSummaryCapabilityPreservesCancellation(t *testing.T) {
	probing := make(chan struct{})
	var posts atomic.Int32
	var block atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathCatalog {
			if block.Load() {
				close(probing)
				<-r.Context().Done()
				return
			}
			_, _ = w.Write([]byte(`{"providers":[]}`))
			return
		}
		posts.Add(1)
	}))
	defer srv.Close()
	p, err := NewClient(srv.URL, testToken, srv.Client()).Resolve(provider.Selection{Ref: "p/m"})
	if err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := p.Stream(ctx, provider.Request{Summary: true, Tools: []provider.ToolSchema{{Name: "read_file"}}})
		done <- err
	}()
	<-probing
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || posts.Load() != 0 {
		t.Fatalf("cancel err=%v posts=%d", err, posts.Load())
	}
}

func TestSummaryCapabilityUsesRotatedTokenAtTheSameURL(t *testing.T) {
	var token atomic.Value
	token.Store("before")
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderToken) != token.Load().(string) {
			t.Error("stale token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == PathCatalog {
			_, _ = w.Write([]byte(`{"providers":[],"summary_tool_choice_none":true}`))
			return
		}
		posts.Add(1)
		_ = json.NewEncoder(w).Encode(encodeChunk(provider.Chunk{Type: provider.ChunkDone}))
	}))
	defer srv.Close()
	c := NewEndpointClient(func() (string, string, error) { return srv.URL, token.Load().(string), nil }, srv.Client())
	p, err := c.Resolve(provider.Selection{Ref: "p/m"})
	if err != nil {
		t.Fatal(err)
	}
	token.Store("after")
	ch, err := p.Stream(t.Context(), provider.Request{Summary: true, Tools: []provider.ToolSchema{{Name: "read_file"}}})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, ch)
	if posts.Load() != 1 {
		t.Fatal("rotated token summary did not reach provider")
	}
}
