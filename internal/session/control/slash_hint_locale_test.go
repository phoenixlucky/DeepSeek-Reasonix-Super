package control

import (
	"testing"

	"reasonix/internal/base/i18n"
)

func TestSlashArgHintsFollowTheCatalog(t *testing.T) {
	lines := []string{
		"/goal ", "/memory ", "/mcp ", "/mcp add x ", "/skills ", "/plugins ", "/theme ",
		"/currency ", "/reasoning-language ", "/ask-timeout ", "/feedback ",
	}
	prev := i18n.M
	t.Cleanup(func() { i18n.M = prev })

	hints := func(cat i18n.Messages) map[string]string {
		i18n.M = cat
		out := map[string]string{}
		for _, line := range lines {
			items, _ := SlashArgItems(line, ArgData{})
			for _, it := range items {
				out[line+it.Label] = it.Hint
			}
		}
		return out
	}
	en, zh, tw := hints(i18n.English), hints(i18n.Chinese), hints(i18n.ChineseTraditional)
	for key, enHint := range en {
		if enHint == "" {
			continue
		}
		if zh[key] == enHint {
			t.Errorf("%s: simplified Chinese hint is still the English text %q", key, enHint)
		}
		if tw[key] == enHint {
			t.Errorf("%s: traditional Chinese hint is still the English text %q", key, enHint)
		}
	}
}
