package control

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/runtime/agent"
)

func namedPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func attachmentWorkspace(t *testing.T) (*Controller, string) {
	t.Helper()
	ws := testenv.TempDir(t)
	dir := filepath.Join(ws, ".reasonix", "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "clipboard-1.png"), namedPNG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	return New(Options{WorkspaceRoot: ws}), ws
}

func TestATaskNamingAnAttachmentGetsItsImage(t *testing.T) {
	c, ws := attachmentWorkspace(t)
	abs := filepath.Join(ws, ".reasonix", "attachments", "clipboard-1.png")
	for name, task := range map[string]string{
		"relative":   "Describe .reasonix/attachments/clipboard-1.png for me",
		"backslash":  `Describe .reasonix\attachments\clipboard-1.png for me`,
		"at-prefix":  "Describe @.reasonix/attachments/clipboard-1.png.",
		"quoted":     `Look at ".reasonix/attachments/clipboard-1.png", then say what is red`,
		"backticked": "Look at `.reasonix/attachments/clipboard-1.png`.",
		"absolute":   "Look at " + abs + " now",
		"bracketed":  "(see [.reasonix/attachments/clipboard-1.png])",
	} {
		t.Run(name, func(t *testing.T) {
			if name == "backslash" && filepath.Separator != '\\' {
				t.Skip("a backslash is a separator only on Windows")
			}
			got := c.attachmentImagesNamedIn(task)
			if len(got) != 1 || !strings.HasPrefix(got[0], "data:image/png;base64,") {
				t.Fatalf("images = %d (%q), want the one attachment", len(got), task)
			}
		})
	}
}

func TestATaskNamingTheSameAttachmentTwiceGetsItOnce(t *testing.T) {
	c, _ := attachmentWorkspace(t)
	task := "Compare .reasonix/attachments/clipboard-1.png with .reasonix/attachments/clipboard-1.png"
	if got := c.attachmentImagesNamedIn(task); len(got) != 1 {
		t.Fatalf("images = %d, want 1", len(got))
	}
}

func TestOnlyAFileDirectlyInTheHostsAttachmentsDirectoryIsAnImageForATask(t *testing.T) {
	c, ws := attachmentWorkspace(t)
	if err := os.MkdirAll(filepath.Join(ws, ".reasonix", "attachments", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"secret.png", filepath.Join(".reasonix", "other.png"), filepath.Join(".reasonix", "attachments", "sub", "deep.png"), filepath.Join(".reasonix", "attachments", "notes.txt")} {
		data := namedPNG(t)
		if strings.HasSuffix(name, ".txt") {
			data = []byte("not an image")
		}
		if err := os.WriteFile(filepath.Join(ws, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, task := range []string{
		"Look at secret.png",
		"Look at .reasonix/other.png",
		"Look at .reasonix/attachments/sub/deep.png",
		"Look at .reasonix/attachments/notes.txt",
		"Look at .reasonix/attachments/../../secret.png",
		"Look at .reasonix/attachments/missing.png",
		"Look at ../elsewhere/.reasonix/attachments/clipboard-1.png",
		"Please read the screenshot the user pasted earlier",
	} {
		if got := c.attachmentImagesNamedIn(task); len(got) != 0 {
			t.Fatalf("%q resolved to %d image(s), want none", task, len(got))
		}
	}
}

func TestAnAttachmentThatIsASymlinkIsNotAnImageForATask(t *testing.T) {
	c, ws := attachmentWorkspace(t)
	outside := filepath.Join(testenv.TempDir(t), "outside.png")
	if err := os.WriteFile(outside, namedPNG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ws, ".reasonix", "attachments", "link.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks are not available here:", err)
	}
	if got := c.attachmentImagesNamedIn("Look at .reasonix/attachments/link.png"); len(got) != 0 {
		t.Fatalf("a symlink in the attachments directory resolved to %d image(s)", len(got))
	}
}

func TestBindTaskImagesJoinsWhatTheTurnCarriedWithWhatTheTaskNames(t *testing.T) {
	c, _ := attachmentWorkspace(t)
	ctx := c.withVisionRouting(agent.WithSubagentImageCandidates(context.Background(), []string{"data:image/png;base64,AAAA"}))
	got := agent.SubagentImageCandidates(agent.BindTaskImages(ctx, "Describe .reasonix/attachments/clipboard-1.png"))
	if len(got) != 2 || got[0] != "data:image/png;base64,AAAA" {
		t.Fatalf("candidates = %d, want the turn's image then the named one", len(got))
	}
	again := agent.SubagentImageCandidates(agent.BindTaskImages(agent.BindTaskImages(ctx, "Describe .reasonix/attachments/clipboard-1.png"), "Describe .reasonix/attachments/clipboard-1.png"))
	if len(again) != 2 {
		t.Fatalf("binding the same task twice gave %d images, want 2", len(again))
	}
	if none := agent.SubagentImageCandidates(agent.BindTaskImages(context.Background(), "x .reasonix/attachments/clipboard-1.png")); len(none) != 0 {
		t.Fatal("a context the controller never touched gained images")
	}
}
