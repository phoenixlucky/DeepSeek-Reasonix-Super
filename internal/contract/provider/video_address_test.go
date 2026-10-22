package provider

import "testing"

func TestOnlyAPublicHTTPSNameIsAnAddressTheVendorMayVisit(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://cdn.example.com/a.mp4":         true,
		"https://www.youtube.com/watch?v=abc":   true,
		"http://cdn.example.com/a.mp4":          false,
		"https://user:pw@cdn.example.com/a.mp4": false,
		"https://127.0.0.1/a.mp4":               false,
		"https://[::1]/a.mp4":                   false,
		"https://10.0.0.5/a.mp4":                false,
		"https://localhost/a.mp4":               false,
		"https://nas/a.mp4":                     false,
		"https://printer.local/a.mp4":           false,
		"https://db.internal/a.mp4":             false,
		"https://a.localhost/a.mp4":             false,
		"https://fileserver.corp/a.mp4":         false,
		"file:///etc/passwd":                    false,
		"https://":                              false,
		"https://cdn.example.com/" + string(make([]byte, MaxVideoAddress)): false,
	} {
		if got := ValidVideoAddress(raw) == nil; got != want {
			t.Errorf("ValidVideoAddress(%.60q) = %v, want %v", raw, got, want)
		}
	}
}

// Zhipu's page shows an HTTPS address; Google's documents YouTube addresses for video.
func TestEachProtocolTakesOnlyTheAddressesItsVendorDocuments(t *testing.T) {
	yt, cdn := "https://youtu.be/abc", "https://cdn.example.com/a.mp4"
	for _, tc := range []struct {
		protocol, addr string
		want           bool
	}{
		{VideoProtocolZhipuURL, cdn, true},
		{VideoProtocolZhipuURL, yt, true},
		{VideoProtocolGeminiFiles, yt, true},
		{VideoProtocolGeminiInline, "https://www.youtube.com/watch?v=x", true},
		{VideoProtocolGeminiFiles, cdn, false},
		{VideoProtocolInlineDataURL, cdn, false},
		{VideoProtocolMoonshotFile, yt, false},
		{"", cdn, false},
	} {
		if got := VideoAddressAccepted(tc.protocol, tc.addr); got != tc.want {
			t.Errorf("%s takes %s: %v, want %v", tc.protocol, tc.addr, got, tc.want)
		}
	}
}

func TestOnlyAGrantedAddressIsSent(t *testing.T) {
	const a = "https://cdn.example.com/granted.mp4"
	if VideoAddressGranted(a) {
		t.Fatal("granted before anyone granted it")
	}
	GrantVideoAddress(a)
	if !VideoAddressGranted(a) || VideoAddressGranted(a+"?x=1") {
		t.Fatal("the grant is for exactly the address checked")
	}
}

func TestTheZhipuProtocolSendsNoFile(t *testing.T) {
	if VideoProtocolTakesFiles(VideoProtocolZhipuURL) || !VideoProtocolTakesFiles(VideoProtocolInlineDataURL) {
		t.Fatal("only the Zhipu protocol is address-only")
	}
}
