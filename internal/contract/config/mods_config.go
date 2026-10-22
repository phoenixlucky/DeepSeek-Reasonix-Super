package config

import (
	"fmt"
	"strings"
)

// ModsConfig governs Claude Code mods: plugins whose hooks module runs in this
// host. Running a module is the install-time trust a code extension already has,
// so the switch is the user's alone and a repository cannot turn it on.
type ModsConfig struct {
	// Enabled runs the hooks module of installed Claude Code mods. Off by default;
	// the user file only.
	Enabled bool `toml:"enabled"`
	// HookBudgetMS bounds one hook's own running time; 0 keeps Claude Code's 10 s.
	// User file only.
	HookBudgetMS int `toml:"hook_budget_ms"`
	// ContextBytes and ContextBlocks bound the text a mod adds to a turn, per
	// block and per turn; the tail is the context window and the bill. 0 removes
	// a ceiling and is the user's explicit choice. User file only.
	ContextBytes  int `toml:"context_bytes"`
	ContextBlocks int `toml:"context_blocks"`
	// MessagesMax is how many transcript items one read by a mod may return, 0
	// for no ceiling. User file only.
	MessagesMax int `toml:"messages_max"`
	// SubmitPerTurn is how many prompts mods may queue between two prompts you
	// write: it stops a mod feeding the session to itself. 0 removes it, a
	// deliberate act. User file only.
	SubmitPerTurn int `toml:"submit_per_turn"`
	// SubmitBytes bounds one prompt a mod queues; 0 removes the ceiling. User file only.
	SubmitBytes int `toml:"submit_bytes"`
}

// Defaults of the ceilings above.
const (
	DefaultModsContextBytes  = 8192
	DefaultModsContextBlocks = 8
	DefaultModsMessagesMax   = 200
	DefaultModsSubmitPerTurn = 3
	DefaultModsSubmitBytes   = 8192
)

func defaultMods() ModsConfig {
	return ModsConfig{ContextBytes: DefaultModsContextBytes, ContextBlocks: DefaultModsContextBlocks, MessagesMax: DefaultModsMessagesMax, SubmitPerTurn: DefaultModsSubmitPerTurn, SubmitBytes: DefaultModsSubmitBytes}
}

// ModsLimits are the user's ceilings on what a mod may add to a session.
type ModsLimits struct {
	ContextBytes, ContextBlocks, MessagesMax, SubmitPerTurn, SubmitBytes int
}

// ModsLimits reads the ceilings; a nil config answers the defaults.
func (c *Config) ModsLimits() ModsLimits {
	m := defaultMods()
	if c != nil {
		m = c.Mods
	}
	return ModsLimits{ContextBytes: max(m.ContextBytes, 0), ContextBlocks: max(m.ContextBlocks, 0), MessagesMax: max(m.MessagesMax, 0), SubmitPerTurn: max(m.SubmitPerTurn, 0), SubmitBytes: max(m.SubmitBytes, 0)}
}

// ModsEnabled reports whether mods run.
func (c *Config) ModsEnabled() bool { return c != nil && c.Mods.Enabled }

// ModsHookBudgetMS is the user's per-hook time limit in milliseconds, 0 for the default.
func (c *Config) ModsHookBudgetMS() int {
	if c == nil {
		return 0
	}
	return max(c.Mods.HookBudgetMS, 0)
}

// narrowMods puts back the mods settings the project file touched.
func (h heldScope) narrowMods(c *Config) {
	if c.Mods.Enabled != h.mods.Enabled {
		c.ignoreProject("mods.enabled", fmt.Sprintf("%t", c.Mods.Enabled), ProjectUserOnly)
	}
	if c.Mods.HookBudgetMS != h.mods.HookBudgetMS {
		c.ignoreProject("mods.hook_budget_ms", fmt.Sprint(c.Mods.HookBudgetMS), ProjectUserOnly)
	}
	for _, f := range []struct {
		key       string
		have, was int
	}{
		{"mods.context_bytes", c.Mods.ContextBytes, h.mods.ContextBytes},
		{"mods.context_blocks", c.Mods.ContextBlocks, h.mods.ContextBlocks},
		{"mods.messages_max", c.Mods.MessagesMax, h.mods.MessagesMax},
		{"mods.submit_per_turn", c.Mods.SubmitPerTurn, h.mods.SubmitPerTurn},
		{"mods.submit_bytes", c.Mods.SubmitBytes, h.mods.SubmitBytes},
	} {
		if f.have != f.was {
			c.ignoreProject(f.key, fmt.Sprint(f.have), ProjectUserOnly)
		}
	}
	c.Mods = h.mods
}

func renderModsSection(b *strings.Builder, c *Config) {
	custom := c.Mods.ContextBytes != DefaultModsContextBytes || c.Mods.ContextBlocks != DefaultModsContextBlocks ||
		c.Mods.MessagesMax != DefaultModsMessagesMax || c.Mods.SubmitPerTurn != DefaultModsSubmitPerTurn || c.Mods.SubmitBytes != DefaultModsSubmitBytes
	if !c.Mods.Enabled && c.Mods.HookBudgetMS == 0 && !custom {
		return
	}
	b.WriteString("[mods]\n")
	fmt.Fprintf(b, "enabled = %v   # run the hooks module of installed Claude Code mods; full trust, user file only\n", c.Mods.Enabled)
	if c.Mods.HookBudgetMS > 0 {
		fmt.Fprintf(b, "hook_budget_ms = %d   # one hook's own running time; 0 keeps 10000\n", c.Mods.HookBudgetMS)
	}
	if custom {
		fmt.Fprintf(b, "context_bytes = %d   # bytes of one block a mod adds to a turn; 0 for no ceiling\n", c.Mods.ContextBytes)
		fmt.Fprintf(b, "context_blocks = %d   # blocks a mod may add to one turn; 0 for no ceiling\n", c.Mods.ContextBlocks)
		fmt.Fprintf(b, "messages_max = %d   # transcript items one read by a mod may return; 0 for no ceiling\n", c.Mods.MessagesMax)
		fmt.Fprintf(b, "submit_per_turn = %d   # prompts mods may queue between two prompts you write; 0 for no ceiling\n", c.Mods.SubmitPerTurn)
		fmt.Fprintf(b, "submit_bytes = %d   # bytes of one prompt a mod queues; 0 for no ceiling\n", c.Mods.SubmitBytes)
	}
	b.WriteString("\n")
}

// UpdateUserMods applies change to the [mods] table of the user file and saves
// it. A repository's file is never touched: the table is the user's alone.
func UpdateUserMods(change func(*ModsConfig) error) (ModsConfig, error) {
	path := UserConfigPath()
	edit := LoadForEdit(path)
	next := edit.Mods
	if err := change(&next); err != nil {
		return edit.Mods, err
	}
	edit.Mods = next
	if err := edit.SaveTo(path); err != nil {
		return edit.Mods, err
	}
	return next, nil
}

// UserMods is the [mods] table as the user file holds it now.
func UserMods() ModsConfig { return LoadForEdit(UserConfigPath()).Mods }
