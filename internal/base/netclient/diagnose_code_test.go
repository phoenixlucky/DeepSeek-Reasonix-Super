package netclient

import (
	"context"
	"net/http/httptest"
	"testing"
)

// A window in either language has to word each step itself, so every step names
// what it found by identity; the Chinese detail stays for logs and the terminal.
func TestDiagnoseStepsCarryTheirIdentity(t *testing.T) {
	srv := httptest.NewServer(nil)
	live := srv.URL
	srv.Close()

	probes := Diagnose(context.Background(), ProxySpec{Mode: ModeOff}, live)
	want := []struct{ step, code, advice string }{
		{"proxy", "net.proxy.off", ""},
		{"dns", "", ""},
		{"connect", "net.connect.failed", "net.connect.blocked"},
	}
	if len(probes) != len(want) {
		t.Fatalf("probes = %+v", probes)
	}
	for i, w := range want {
		if probes[i].Step != w.step || probes[i].Code != w.code || probes[i].AdviceCode != w.advice {
			t.Errorf("step %d = %s code %q advice %q, want %s %q %q", i, probes[i].Step, probes[i].Code, probes[i].AdviceCode, w.step, w.code, w.advice)
		}
	}
	if probes[2].Params["host"] == nil || probes[2].Params["port"] == nil {
		t.Errorf("a failed connect did not say which address: %+v", probes[2].Params)
	}
}

func TestDiagnoseNamesAnUnreadableAddress(t *testing.T) {
	probes := Diagnose(context.Background(), ProxySpec{Mode: ModeOff}, "http://")
	if len(probes) != 1 || probes[0].Code != "net.dns.unreadable" {
		t.Fatalf("probes = %+v", probes)
	}
}
