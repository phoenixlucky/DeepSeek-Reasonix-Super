package serve

import (
	"bytes"
	"os"
	"testing"

	"reasonix/internal/contract/config"
)

func TestAnUntouchedConfigReadsNoTextSize(t *testing.T) {
	zoomHome(t, "")
	if got := zoomRead(t).TextSize; got != 0 {
		t.Fatalf("an untouched config reads text size %v, want unset", got)
	}
}

func TestSavedTextSizeIsClampedToTheAnnouncedRange(t *testing.T) {
	zoomHome(t, "")
	for _, c := range []struct{ sent, want float64 }{
		{3, config.TextSizeMax},
		{0.4, config.TextSizeMin},
		{1.15, 1.15},
		{0, 0},
	} {
		if got := zoomSave(t, map[string]any{"textSize": c.sent}).TextSize; got != c.want {
			t.Errorf("text size %v saved as %v, want %v", c.sent, got, c.want)
		}
	}
}

func TestTextSizeIsIndependentOfZoomAndLeavesTheConfigAloneAtTheDefault(t *testing.T) {
	zoomHome(t, "")
	zoomSave(t, map[string]any{"zoom": 1.3, "textSize": 1.15})
	got := zoomRead(t)
	if got.Zoom != 1.3 || got.TextSize != 1.15 {
		t.Fatalf("zoom %v and text size %v, want both kept apart", got.Zoom, got.TextSize)
	}
	zoomSave(t, map[string]any{"zoom": 1.3, "textSize": 0})
	raw, err := os.ReadFile(config.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("text_size")) {
		t.Fatalf("the default text size was written to the config:\n%s", raw)
	}
}
