package config

import (
	"errors"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestSessionRefTokensDefaultsAndHoldsToItsBounds(t *testing.T) {
	var none *Config
	if got := none.SessionRefTokens(); got != DefaultSessionRefTokens {
		t.Fatalf("nil config = %d, want the default %d", got, DefaultSessionRefTokens)
	}
	for set, want := range map[int]int{
		0: DefaultSessionRefTokens, -5: DefaultSessionRefTokens, 100: SessionRefTokensMin,
		12000: 12000, 9_000_000: SessionRefTokensMax,
	} {
		c := &Config{}
		c.Desktop.SessionRefTokens = set
		if got := c.SessionRefTokens(); got != want {
			t.Fatalf("session_ref_tokens = %d reads as %d, want %d", set, got, want)
		}
	}
}

func TestSessionRefTokensSurvivesSaveAndLoad(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "config.toml")
	cfg := LoadForEdit(path)
	cfg.Desktop.SessionRefTokens = 12000
	if err := cfg.SaveToScope(path, RenderScopeUser); err != nil {
		t.Fatal(err)
	}
	back := LoadForEdit(path)
	if got := back.SessionRefTokens(); got != 12000 {
		t.Fatalf("after a save and load the limit is %d, want 12000", got)
	}
	if err := back.SaveToScope(path, RenderScopeUser); err != nil {
		t.Fatal(err)
	}
	if again := LoadForEdit(path); again.SessionRefTokens() != 12000 {
		t.Fatalf("a second save lost the limit: %d", again.SessionRefTokens())
	}
}

func TestSetSessionRefTokensRefusesWhatReadsAsSomethingElse(t *testing.T) {
	c := &Config{}
	for _, bad := range []int{-1, 1, SessionRefTokensMin - 1, SessionRefTokensMax + 1} {
		if err := c.SetSessionRefTokens(bad); !errors.Is(err, ErrSessionRefTokensOutOfRange) {
			t.Fatalf("%d: err = %v, want out of range", bad, err)
		}
		if c.Desktop.SessionRefTokens != 0 {
			t.Fatalf("%d was stored despite the refusal", bad)
		}
	}
	if err := c.SetSessionRefTokens(9000); err != nil || c.SessionRefTokens() != 9000 {
		t.Fatalf("a valid limit: err=%v reads %d", err, c.SessionRefTokens())
	}
	if err := c.SetSessionRefTokens(0); err != nil || c.SessionRefTokens() != DefaultSessionRefTokens {
		t.Fatalf("0 must return to the default: err=%v reads %d", err, c.SessionRefTokens())
	}
}
