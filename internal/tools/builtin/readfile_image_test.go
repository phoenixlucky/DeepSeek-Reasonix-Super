package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// An image on disk cannot be read as text, and the refusal points at the routes
// that work (view_image where offered, the user attaching it) instead of
// promising a delegation that would reach a child with no picture.
func TestReadFileImageRefusalPointsAtTheRoutesThatWork(t *testing.T) {
	dir := testenv.TempDir(t)
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "shot.png"})
	_, err := readFile{workDir: dir}.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("an image was returned as text")
	}
	msg := err.Error()
	for _, want := range []string{"PNG image", "view_image", "references it as @", "shot.png", "ask them"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("refusal lacks %q: %s", want, msg)
		}
	}
}
