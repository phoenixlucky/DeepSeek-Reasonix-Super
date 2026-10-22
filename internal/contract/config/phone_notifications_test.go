package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestPhoneNotificationsRoundTripAndAreOffByDefault(t *testing.T) {
	isolateUserConfigHome(t)
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	if err := os.WriteFile(path, []byte("default_model = \"deepseek\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	if cfg.Notifications.Phone != (PhoneNotificationsConfig{}) {
		t.Fatalf("phone notifications are on by default: %+v", cfg.Notifications.Phone)
	}
	cfg.Notifications.Phone = PhoneNotificationsConfig{Enabled: true, Approval: true, Error: true}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "[notifications.phone]") {
		t.Fatalf("the saved config has no phone table:\n%s", raw)
	}
	got := LoadForEdit(path).Notifications.Phone
	if want := (PhoneNotificationsConfig{Enabled: true, Approval: true, Error: true}); got != want {
		t.Fatalf("reloaded = %+v, want %+v", got, want)
	}
}
