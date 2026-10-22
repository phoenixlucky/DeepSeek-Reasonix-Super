package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestServeIndexPageAndSessionDeepLink(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	server := New(ctrl, bc, config.ServeConfig{})
	server.page = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<title>studio</title>")}}

	// A deep link names a session, and the page is what opens it. Both answer
	// with the same shell and keep the path, so the page can read it.
	for _, path := range []string{"/", "/sessions/reserved-session"} {
		rec := httptest.NewRecorder()
		operatorHandler(server).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>studio</title>") {
			t.Errorf("GET %s = %d %q, want the page shell", path, rec.Code, rec.Body.String())
		}
	}
}

// The built Studio page is the interface. A kernel that drew a second one at /
// shipped two, and the older one was the one a browser landed on.
func TestRootHandsTheVisitorToTheBuiltPage(t *testing.T) {
	bc := NewBroadcaster()
	srv := New(control.New(control.Options{Sink: bc}), bc, config.ServeConfig{})
	srv.page = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<title>studio</title>")}}
	rec := httptest.NewRecorder()
	operatorHandler(srv).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>studio</title>") {
		t.Fatalf("GET / = %d %q, want the page shell", rec.Code, rec.Body.String())
	}
}

// With nothing mounted the redirect would come straight back here, so the
// kernel says what is missing instead of looping a browser.
func TestRootSaysSoWhenNoPageIsBuilt(t *testing.T) {
	bc := NewBroadcaster()
	srv := New(control.New(control.Options{Sink: bc}), bc, config.ServeConfig{})
	rec := httptest.NewRecorder()
	operatorHandler(srv).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET / with no page = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "page.not_built") {
		t.Fatalf("body = %q, want the code a frontend can act on", rec.Body.String())
	}
}

// A browser that lands here has nothing to do with the JSON a client reads; it
// is told, in a page, what this server is and how to get the interface. The
// answer is asked of the Accept header's media ranges, not of the user agent.
func TestARequestForHTMLGetsAReadableExplanationWhenNoPageIsBuilt(t *testing.T) {
	cases := []struct {
		name, accept string
		wantHTML     bool
	}{
		{"browser navigation", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", true},
		{"html only", "text/html", true},
		{"json client", "application/json", false},
		{"curl default", "*/*", false},
		{"no header", "", false},
		{"html refused", "text/html;q=0, application/json", false},
		{"json preferred over html", "application/json, text/html;q=0.5", false},
		{"html preferred over json", "application/json;q=0.4, text/html", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bc := NewBroadcaster()
			srv := New(control.New(control.Options{Sink: bc}), bc, config.ServeConfig{})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			operatorHandler(srv).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 either way", rec.Code)
			}
			isHTML := strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html")
			if isHTML != tc.wantHTML {
				t.Fatalf("Content-Type = %q, want html = %v", rec.Header().Get("Content-Type"), tc.wantHTML)
			}
			if tc.wantHTML {
				body := rec.Body.String()
				if !strings.Contains(body, "--page") || !strings.Contains(body, "frontend-next") {
					t.Fatalf("the page does not say how to get the interface:\n%s", body)
				}
				if strings.Contains(body, "page.not_built") && !strings.Contains(body, "<html") {
					t.Fatalf("an HTML request received the JSON body: %s", body)
				}
			} else if !strings.Contains(rec.Body.String(), "page.not_built") {
				t.Fatalf("a JSON client lost its code: %q", rec.Body.String())
			}
		})
	}
}

// The shell names which build is current and the page's own out-of-date check
// reads this same file. A stored copy makes an open page look behind, and
// reloading would land on that same copy, which is what the hashed bundles under
// /assets avoid by being named for their content and never reused.
func TestThePageShellIsNeverStored(t *testing.T) {
	bc := NewBroadcaster()
	srv := New(control.New(control.Options{Sink: bc}), bc, config.ServeConfig{})
	srv.page = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<title>studio</title>")}}
	for _, path := range []string{"/", "/sessions/reserved-session"} {
		rec := httptest.NewRecorder()
		operatorHandler(srv).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Result().Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, got)
		}
	}
}
