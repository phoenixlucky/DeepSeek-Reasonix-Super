package boot

import (
	"runtime"
	"testing"
)

// A host that ships no browser binary and names none has no browser tools to
// give: every call would end in browser.engine_missing, which asks the reader to
// install a browser the platform cannot install. Such a host keeps a WebView,
// which is not a process the browser pool can launch and drive.
func TestBrowserWithoutEngine(t *testing.T) {
	if !browserWithoutEngine("android", "") {
		t.Fatal("a platform with no browser binary and none named should have no browser tools")
	}
	// A configured executable is taken at its word: the host that says where its
	// browser is has one, and the pool decides the rest at first use.
	if browserWithoutEngine("android", "/data/local/tmp/chromium") {
		t.Fatal("a configured executable should keep the browser tools")
	}
	// Whitespace is not a path.
	if !browserWithoutEngine("android", "   ") {
		t.Fatal("a blank executable is the same as none")
	}
	// Every other platform keeps the existing behavior: the tools are bound and
	// any failure is discovered when the pool is first used.
	for _, goos := range []string{"linux", "darwin", "windows"} {
		if browserWithoutEngine(goos, "") {
			t.Fatalf("%s should keep its browser tools", goos)
		}
	}
}

// The check is on the platform it is handed, not on the machine running the
// test.
func TestBrowserWithoutEngineIgnoresThisHost(t *testing.T) {
	if browserWithoutEngine(runtime.GOOS, "") && runtime.GOOS != "android" {
		t.Fatalf("this host (%s) has no engine but should still bind the tools", runtime.GOOS)
	}
}
