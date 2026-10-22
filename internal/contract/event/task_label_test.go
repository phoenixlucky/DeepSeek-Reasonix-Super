package event

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTaskLabelPrefersTheDescriptionThenTheFirstLineOfTheTask(t *testing.T) {
	if got := TaskLabel("probe the cache", "something else\nmore"); got != "probe the cache" {
		t.Fatalf("label = %q, want the description", got)
	}
	if got := TaskLabel("  ", "\n\n  Find the loader  \nthen more"); got != "Find the loader" {
		t.Fatalf("label = %q, want the task's first non-blank line", got)
	}
	if got := TaskLabel("", ""); got != "" {
		t.Fatalf("label = %q, want none", got)
	}
}

func TestTaskLabelIsBoundedInCharactersNotBytes(t *testing.T) {
	got := TaskLabel("", strings.Repeat("查", 500))
	if n := utf8.RuneCountInString(got); n != MaxTaskLabel || !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Fatalf("label has %d runes (valid %v), want %d ending in an ellipsis", n, utf8.ValidString(got), MaxTaskLabel)
	}
}
