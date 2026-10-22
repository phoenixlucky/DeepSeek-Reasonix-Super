package serve

import "testing"

func TestVendorOfKeepsThePortThatNamesAnotherServer(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"http://127.0.0.1:8788/v1", "127.0.0.1:8788"},
		{"http://127.0.0.1:8756/v1", "127.0.0.1:8756"},
		{"https://API.DeepSeek.com/v1", "api.deepseek.com"},
		{"https://api.deepseek.com:443/v1", "api.deepseek.com"},
		{"http://relay.example:80", "relay.example"},
		{"http://[::1]:9000/v1", "[::1]:9000"},
		{"  http://localhost:11434/v1 ", "localhost:11434"},
	} {
		if got := vendorOf(tc.url); got != tc.want {
			t.Errorf("vendorOf(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestKeyReuseStopsAtThePort(t *testing.T) {
	if vendorOf("http://127.0.0.1:8788/v1") == vendorOf("http://127.0.0.1:8756/v1") {
		t.Fatal("two ports on one host are two servers and must not share a key slot")
	}
}
