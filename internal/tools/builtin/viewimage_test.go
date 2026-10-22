package builtin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/model/visionimage"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func viewImageFor(dir string) viewImage {
	roots := realRoots([]string{dir})
	return viewImage{workDir: dir, readRoots: roots}
}

func viewArgs(path string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"path": path})
	return b
}

// The picture itself comes back on the image channel, in the type its bytes
// have, with a text line saying what was shown.
func TestViewImageReturnsThePictureItself(t *testing.T) {
	dir := testenv.TempDir(t)
	writePNG(t, filepath.Join(dir, "shot.png"), 40, 20)
	text, images, err := viewImageFor(dir).ExecuteWithImages(context.Background(), viewArgs("shot.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || !strings.HasPrefix(images[0], "data:image/png;base64,") {
		t.Fatalf("images = %v, want one PNG data URL", images)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(images[0], "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(raw)); err != nil || cfg.Width != 40 || cfg.Height != 20 {
		t.Fatalf("returned picture is %v, %v; want the 40x20 original", cfg, err)
	}
	if !strings.Contains(text, "shot.png") || !strings.Contains(text, "image/png") {
		t.Fatalf("text = %q, want the path and type", text)
	}
	if !(viewImage{}).ReadOnly() {
		t.Fatal("viewing a file is a read")
	}
}

// A picture larger than a vision host accepts is fitted, not forwarded.
func TestViewImageFitsAnOversizedPicture(t *testing.T) {
	dir := testenv.TempDir(t)
	writePNG(t, filepath.Join(dir, "wide.png"), visionimage.MaxDim*2, 8)
	_, images, err := viewImageFor(dir).ExecuteWithImages(context.Background(), viewArgs("wide.png"))
	if err != nil {
		t.Fatal(err)
	}
	_, payload, _ := strings.Cut(images[0], ",")
	raw, _ := base64.StdEncoding.DecodeString(payload)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width > visionimage.MaxDim {
		t.Fatalf("sent %v (%v), want a width within %d", cfg, err, visionimage.MaxDim)
	}
}

// The type is the bytes', never the name's: text wearing .png is refused with
// its identity, and the real type wins over a wrong extension.
func TestViewImageDecidesByBytesAndRefusesWithIdentities(t *testing.T) {
	dir := testenv.TempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "fake.png"), []byte("just some text, not a picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, "real.txt"), 4, 4)
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	v := viewImageFor(dir)
	if _, _, err := v.ExecuteWithImages(context.Background(), viewArgs("fake.png")); refusalCode(err) != CodeImageUnsupported {
		t.Fatalf("text named .png = %v, want %s", err, CodeImageUnsupported)
	}
	if _, images, err := v.ExecuteWithImages(context.Background(), viewArgs("real.txt")); err != nil || len(images) != 1 {
		t.Fatalf("a PNG named .txt = %v, %v; want it shown", images, err)
	}
	if _, _, err := v.ExecuteWithImages(context.Background(), viewArgs("folder")); err == nil {
		t.Fatal("a directory was shown")
	}
	small := viewImageFor(dir)
	small.maxBytes = 10
	if _, _, err := small.ExecuteWithImages(context.Background(), viewArgs("real.txt")); refusalCode(err) != CodeImageTooLarge {
		t.Fatalf("a file past the bound = %v, want %s", err, CodeImageTooLarge)
	}
	if _, _, err := v.ExecuteWithImages(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("no path was accepted")
	}
}

// The read boundary is read_file's: nothing outside the roots, nothing forbidden.
func TestViewImageStaysInsideTheReadBoundary(t *testing.T) {
	dir, outside, secret := testenv.TempDir(t), testenv.TempDir(t), testenv.TempDir(t)
	writePNG(t, filepath.Join(outside, "elsewhere.png"), 4, 4)
	writePNG(t, filepath.Join(secret, "key.png"), 4, 4)
	v := viewImageFor(dir)
	if _, _, err := v.ExecuteWithImages(context.Background(), viewArgs(filepath.Join(outside, "elsewhere.png"))); err == nil {
		t.Fatal("a picture outside the read roots was shown")
	}
	forbidding := viewImage{workDir: dir, readRoots: realRoots([]string{dir, secret}), forbidRoots: realRoots([]string{secret})}
	if _, _, err := forbidding.ExecuteWithImages(context.Background(), viewArgs(filepath.Join(secret, "key.png"))); err == nil {
		t.Fatal("a picture under a forbidden-read root was shown")
	}
}
