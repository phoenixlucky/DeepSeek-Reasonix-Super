package builtin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/base/netclient"
)

func localFetcher() LocalFetcher {
	return NewLocalFetcher(netclient.ProxySpec{Mode: netclient.ModeOff})
}

// The status is a fact of the response, read off it, so a caller deciding
// whether the page answered never has to look inside the rendered text.
func TestFetchPageCarriesTheStatusAndText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body><h1>Gone</h1></body></html>"))
	}))
	defer srv.Close()
	page, err := localFetcher().Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if page.Status != http.StatusNotFound || !strings.Contains(page.Text, "Gone") || page.ContentType == "" {
		t.Fatalf("page = %+v", page)
	}
}

func TestFetchPageEmptyBodyIsEmptyText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	page, err := localFetcher().Fetch(context.Background(), srv.URL)
	if err != nil || page.Status != http.StatusOK || page.Text != "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

// What web_fetch printed before is what it prints now.
func TestRenderedPageKeepsTheToolOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	page, _ := localFetcher().Fetch(context.Background(), srv.URL)
	if got := page.Rendered(); got != "status 200 OK · text/plain · 5 bytes\n\nhello" {
		t.Fatalf("rendered = %q", got)
	}
	empty := FetchPage{StatusText: "204 No Content"}
	if got := empty.Rendered(); got != "(empty body — status 204 No Content)" {
		t.Fatalf("rendered empty = %q", got)
	}
}

// A refusal to dial an internal address is an identity, not a sentence, so a
// caller can tell a policy refusal from a site that did not answer.
func TestInternalAddressRefusalIsTyped(t *testing.T) {
	_, err := localFetcher().Fetch(context.Background(), "http://169.254.169.254/latest/meta-data/")
	if !errors.Is(err, ErrFetchInternalAddress) {
		t.Fatalf("err = %v, want ErrFetchInternalAddress", err)
	}
	_, err = localFetcher().Fetch(context.Background(), "http://127.0.0.1:1/")
	if errors.Is(err, ErrFetchInternalAddress) {
		t.Fatalf("a refused connection to loopback was reported as a policy refusal: %v", err)
	}
}

// Before a URL is handed to a third party it is checked here: nothing that
// names this machine's own network is sent anywhere.
func TestVetFetchURLKeepsInternalAddressesAtHome(t *testing.T) {
	stubFetchLookup(t, "10.1.2.3")
	for _, raw := range []string{
		"http://169.254.169.254/latest",
		"http://10.0.0.5/admin",
		"http://127.0.0.1:8080/",
		"http://[::1]/",
		"http://localhost:3000/",
		"https://intranet.example/",
	} {
		if err := VetFetchURL(context.Background(), raw); !errors.Is(err, ErrFetchInternalAddress) {
			t.Errorf("%s: err = %v, want ErrFetchInternalAddress", raw, err)
		}
	}
	for _, raw := range []string{"ftp://x/", "http://", "not a url", "https://u:p@example.com/"} {
		if err := VetFetchURL(context.Background(), raw); err == nil || errors.Is(err, ErrFetchInternalAddress) {
			t.Errorf("%s: err = %v, want a plain invalid-address error", raw, err)
		}
	}
}

func TestVetFetchURLAllowsPublicAndUnresolvableNames(t *testing.T) {
	stubFetchLookup(t, "93.184.216.34")
	if err := VetFetchURL(context.Background(), "https://example.com/page?q=1"); err != nil {
		t.Fatalf("a public name was refused: %v", err)
	}
	prev := lookupFetchHost
	t.Cleanup(func() { lookupFetchHost = prev })
	lookupFetchHost = func(context.Context, string) ([]net.IPAddr, error) { return nil, errors.New("no such host") }
	if err := VetFetchURL(context.Background(), "https://blocked-locally.org/"); err != nil {
		t.Fatalf("a name this machine cannot resolve was refused, which is the case a remote fetch exists for: %v", err)
	}
}

// A name this machine cannot resolve may still be an internal one: off the
// company VPN, jira.corp.internal does not resolve either. What is refused is
// decided by the shape of the name, from the standard special-use sets, so the
// hostname, the path and the query token of an internal address stay home.
func TestVetFetchURLRefusesNamesThatAreNotPublicByConstruction(t *testing.T) {
	prev := lookupFetchHost
	t.Cleanup(func() { lookupFetchHost = prev })
	lookupFetchHost = func(context.Context, string) ([]net.IPAddr, error) { return nil, errors.New("no such host") }
	for _, raw := range []string{
		"https://jira/browse/ABC-1?token=secret",
		"http://printer.local/status",
		"https://jira.corp.internal/browse/ABC-1?token=secret",
		"http://router.lan/",
		"http://nas.home.arpa/",
		"https://thing.invalid/",
		"https://thing.test/",
		"https://thing.example/",
		"http://abcdef.onion/",
		"http://LOCALHOST/",
		"http://x.localhost./",
		"http://Printer.LOCAL./",
	} {
		if err := VetFetchURL(context.Background(), raw); !errors.Is(err, ErrFetchInternalAddress) {
			t.Errorf("%s: err = %v, want ErrFetchInternalAddress", raw, err)
		}
	}
	for _, raw := range []string{
		"https://blocked-by-the-firewall.org/page",
		"https://docs.example.com/",
		"https://news.example.net/story?id=1",
		"https://locally-hosted.com/",
		"https://internal-tools.io/",
	} {
		if err := VetFetchURL(context.Background(), raw); err != nil {
			t.Errorf("%s: a public-shaped name was refused: %v", raw, err)
		}
	}
}
