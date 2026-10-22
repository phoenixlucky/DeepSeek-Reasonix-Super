package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/tool"
)

// A call another tool makes reads the text itself: whatever the file holds
// after decoding, with no line numbers, trailer or default page, and offset and
// limit selecting whole lines with their own endings.
func TestReadFileNestedReturnsTheText(t *testing.T) {
	dir := testenv.TempDir(t)
	write := func(name string, body []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	long := strings.Repeat("x\n", readFileDefaultLimit+5)
	crlf := write("crlf.txt", []byte("one\r\ntwo\r\nthree"))
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"crlf whole", map[string]any{"path": crlf}, "one\r\ntwo\r\nthree"},
		{"bom stripped", map[string]any{"path": write("bom.txt", []byte("\xef\xbb\xbfhi\n"))}, "hi\n"},
		{"empty", map[string]any{"path": write("empty.txt", nil)}, ""},
		{"past the default page", map[string]any{"path": write("long.txt", []byte(long))}, long},
		{"window", map[string]any{"path": crlf, "offset": 1, "limit": 1}, "two\r\n"},
		{"last line", map[string]any{"path": crlf, "offset": 2}, "three"},
		{"past eof", map[string]any{"path": crlf, "offset": 9}, ""},
	}
	ctx := tool.MarkNested(t.Context())
	for _, c := range cases {
		got, err := readFile{}.Execute(ctx, argsJSON(t, c.args))
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	if out := runTool(t, readFile{}, map[string]any{"path": crlf}); !strings.Contains(out, "1→one") {
		t.Errorf("the model's own read lost its line numbers: %q", out)
	}
}
