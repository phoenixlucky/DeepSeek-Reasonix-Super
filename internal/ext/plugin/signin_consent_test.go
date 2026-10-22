package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestAuthorizationHostIsTheHostWithItsPortAndNothingElse(t *testing.T) {
	cases := map[string]string{
		"https://Auth.Example.test/authorize?x=1": "auth.example.test",
		"https://auth.example.test:8443/a":        "auth.example.test:8443",
		"https://user:pw@auth.example.test/a":     "auth.example.test",
		"/relative":                               "",
		"not a url":                               "",
		"":                                        "",
	}
	for raw, want := range cases {
		if got := AuthorizationHost(raw); got != want {
			t.Errorf("AuthorizationHost(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSignInConsentIsPerHostPrivateAndWithdrawable(t *testing.T) {
	dir := filepath.Join(testenv.TempDir(t), "figma")
	if SignInHostRemembered(dir, "auth.example.test") {
		t.Fatal("a server nobody agreed for is remembered")
	}
	if err := RememberSignInHost(dir, "Auth.Example.test"); err != nil {
		t.Fatal(err)
	}
	if err := RememberSignInHost(dir, "auth.example.test"); err != nil {
		t.Fatal(err)
	}
	if !SignInHostRemembered(dir, "auth.example.test") || SignInHostRemembered(dir, "evil.example.test") {
		t.Fatal("consent is not bound to the host the user saw")
	}
	if got := readSignInConsent(dir).Hosts; len(got) != 1 {
		t.Fatalf("hosts = %v, want one entry", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(signInConsentPath(dir))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("consent file mode: %v %v", info, err)
		}
	}
	if err := ForgetSignInConsent(dir); err != nil || SignInHostRemembered(dir, "auth.example.test") {
		t.Fatalf("forget: %v", err)
	}
	if err := ForgetSignInConsent(dir); err != nil {
		t.Fatalf("forgetting nothing failed: %v", err)
	}
}

func TestACorruptConsentFileGrantsNothing(t *testing.T) {
	dir := testenv.TempDir(t)
	if err := os.WriteFile(signInConsentPath(dir), []byte(`{"version":2,"hosts":["auth.example.test"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if SignInHostRemembered(dir, "auth.example.test") {
		t.Fatal("an unknown file version was honoured")
	}
	if err := os.WriteFile(signInConsentPath(dir), []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if SignInHostRemembered(dir, "auth.example.test") {
		t.Fatal("a malformed file was honoured")
	}
}
