package websearch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func TestForbiddenAddressesAreDecidedByClassNotSpelling(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"169.254.169.254", true}, {"169.254.0.1", true}, {"fe80::1", true}, {"::ffff:169.254.169.254", true},
		{"100.100.100.200", true}, {"fd00:ec2::254", true}, {"0.0.0.0", true}, {"0.1.2.3", true}, {"::", true},
		{"224.0.0.1", true}, {"ff02::1", true}, {"255.255.255.255", true},
		{"127.0.0.1", false}, {"::1", false}, {"10.0.0.5", false}, {"192.168.1.20", false}, {"172.16.0.9", false},
		{"fd12:3456::1", false}, {"100.64.0.7", false}, {"93.184.216.34", false},
	} {
		if got := forbiddenAddr(netip.MustParseAddr(tc.addr)); got != tc.want {
			t.Errorf("forbiddenAddr(%s) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func lookupTo(addrs ...string) (lookupFunc, *atomic.Int32) {
	var calls atomic.Int32
	return func(context.Context, string) ([]netip.Addr, error) {
		calls.Add(1)
		var out []netip.Addr
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out, nil
	}, &calls
}

// A name that resolves to the metadata endpoint is refused before any connection
// is made, whatever the name looks like.
func TestProbeRefusesAHostnameThatResolvesToAForbiddenAddress(t *testing.T) {
	lookup, _ := lookupTo("169.254.169.254")
	got, err := probeSearxng(context.Background(), "http://searx.example.test:8080", lookup)
	if err != nil || got.OK || got.Code != CodeSearxngAddressRefused {
		t.Fatalf("probe = %+v, %v", got, err)
	}
}

func TestProbeRefusesALiteralLinkLocalAddress(t *testing.T) {
	got, err := ProbeSearxng(context.Background(), "http://169.254.169.254/latest")
	if err != nil || got.Code != CodeSearxngAddressRefused {
		t.Fatalf("probe = %+v, %v", got, err)
	}
}

// One bad answer among good ones refuses the lot: the dialer must not be able to
// pick the harmless one for the check and the other for the connection.
func TestProbeRefusesWhenAnyAnswerIsForbidden(t *testing.T) {
	lookup, _ := lookupTo("127.0.0.1", "169.254.169.254")
	got, _ := probeSearxng(context.Background(), "http://mixed.example.test:8080", lookup)
	if got.Code != CodeSearxngAddressRefused {
		t.Fatalf("probe = %+v", got)
	}
}

// The connection goes to the address that was checked, by address: the name is
// resolved once and the request still carries the name the user typed.
func TestProbeConnectsToTheCheckedAddressAndKeepsTheHostName(t *testing.T) {
	var host atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host.Store(r.Host)
		_, _ = w.Write([]byte(`{"results":[{"title":"t","url":"https://example.com","content":"c"}]}`))
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	lookup, calls := lookupTo("127.0.0.1")
	got, err := probeSearxng(context.Background(), "http://searx.home.test:"+port, lookup)
	if err != nil || !got.OK {
		t.Fatalf("probe = %+v, %v", got, err)
	}
	if calls.Load() != 1 || host.Load() != "searx.home.test:"+port {
		t.Fatalf("lookups = %d, Host = %v", calls.Load(), host.Load())
	}
}

// A self-hosted instance on the loopback or a private network is the normal case.
func TestProbeAllowsLoopbackAndPrivateInstances(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"results":[]}`)) }))
	defer srv.Close()
	got, err := ProbeSearxng(context.Background(), srv.URL)
	if err != nil || !got.OK {
		t.Fatalf("probe = %+v, %v", got, err)
	}
}
