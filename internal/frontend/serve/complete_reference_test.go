package serve

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf16"

	"reasonix/internal/base/testenv"
	"reasonix/internal/session/control"
)

func TestCompleteReferenceHTTPDropsTypedPaths(t *testing.T) {
	root := testenv.TempDir(t)
	for _, name := range []string{"REASONIX.md", "notes.md", "notes.md.bak", "my dir/inner.md"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctrl := control.New(control.Options{SessionDir: testenv.TempDir(t), WorkspaceRoot: root})
	t.Cleanup(func() { ctrl.Close() })
	for _, tc := range []struct {
		name string
		line string
		want []string
	}{
		{name: "partial filename", line: "@REAS", want: []string{"@REASONIX.md"}},
		{name: "exact filename", line: "@REASONIX.md"},
		{name: "exact filename in prose", line: "检查 @REASONIX.md"},
		{name: "exact filename after emoji", line: "📂 @REASONIX.md"},
		{name: "escaped nested path", line: `@my\ dir/inner.md`},
		{name: "genuine alternative", line: "@notes.md", want: []string{"@notes.md.bak"}},
		{name: "exact final alternative", line: "@notes.md.bak"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := completeAt(t, ctrl, tc.line, len(utf16.Encode([]rune(tc.line))))
			inserts := make([]string, 0, len(got.Items))
			for _, item := range got.Items {
				inserts = append(inserts, item.Insert)
			}
			if !slices.Equal(inserts, tc.want) {
				t.Fatalf("GET /complete for %q returned %q, want %q", tc.line, inserts, tc.want)
			}
			if got.Items == nil {
				t.Fatal("GET /complete must encode empty candidates as an array")
			}
			if len(tc.want) == 0 && got.Kind != "" {
				t.Fatalf("GET /complete returned kind %q without candidates", got.Kind)
			}
			if len(tc.want) > 0 && got.Kind != control.CompleteRef {
				t.Fatalf("GET /complete returned kind %q, want a reference menu", got.Kind)
			}
		})
	}
}
