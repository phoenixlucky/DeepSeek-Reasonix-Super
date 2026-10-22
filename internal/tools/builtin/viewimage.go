package builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"reasonix/internal/contract/tool"
	"reasonix/internal/model/visionimage"
)

// Identities of the refusals view_image owns, so a caller never classifies on a
// sentence.
const (
	CodeImageUnsupported = "image.unsupported_type"
	CodeImageTooLarge    = "image.too_large"
	CodeImageUnfit       = "image.unfit"
)

// viewImageMaxBytes bounds the file read before it is decoded.
const viewImageMaxBytes = int64(20) << 20

// ViewImage is the view_image tool bound to the workspace's read boundary. It is
// not a global built-in: a session offers it only when its model reads images,
// so every other session's tool block stays as it was.
func (w Workspace) ViewImage() tool.Tool {
	readRoots := realRoots(w.ReadRoots)
	if len(w.ReadRoots) > 0 && len(readRoots) == 0 {
		readRoots = []string{filepath.Join(string(filepath.Separator), "\x00unresolved")}
	}
	return viewImage{workDir: w.Dir, netRoots: networkRoots(w.Dir, w.WriteRoots, w.ReadRoots), readRoots: readRoots,
		paths: w.ReadPaths, forbidRoots: realRoots(w.ForbidReadRoots)}
}

// viewImage shows the model a picture that is a file in the workspace. It reads
// one confined file, decides its type from its bytes, fits it to what vision
// hosts accept, and returns it on the tool's image channel.
type viewImage struct {
	workDir     string
	netRoots    []string
	readRoots   []string
	paths       *PathResolver
	forbidRoots []string
	// maxBytes is for tests; zero means the real bound.
	maxBytes int64
}

func (viewImage) Name() string   { return "view_image" }
func (viewImage) ReadOnly() bool { return true }

func (viewImage) Description() string {
	return "Look at an image file (PNG, JPEG, GIF or WebP) in the workspace: the picture itself is shown to you, not a description. Needs a model that reads images."
}

func (viewImage) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`)
}

func (v viewImage) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	text, _, err := v.ExecuteWithImages(ctx, args)
	return text, err
}

func (v viewImage) ExecuteWithImages(_ context.Context, args json.RawMessage) (string, []string, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return "", nil, errors.New("path is required")
	}
	rp := resolveReadablePath(v.workDir, p.Path, v.paths)
	if err := v.refuse(rp); err != nil {
		return "", nil, err
	}
	raw, err := v.read(rp)
	if err != nil {
		return "", nil, err
	}
	mime := visionimage.DetectMime(raw)
	if mime == "" {
		return "", nil, tool.Refusal{Code: CodeImageUnsupported, Message: fmt.Sprintf("%s is not a PNG, JPEG, GIF or WebP image (its bytes look like %q)", rp.DisplayPath, visionimage.Sniff(raw))}
	}
	fitted, fittedMime, err := visionimage.Fit(raw, mime)
	if err != nil {
		return "", nil, tool.Refusal{Code: CodeImageUnfit, Message: fmt.Sprintf("%s cannot be shown: %v", rp.DisplayPath, err)}
	}
	text := fmt.Sprintf("[image: %s, %s, %d bytes shown]", rp.DisplayPath, fittedMime, len(fitted))
	return text, []string{"data:" + fittedMime + ";base64," + base64.StdEncoding.EncodeToString(fitted)}, nil
}

func (v viewImage) refuse(rp ResolvedPath) error {
	if err := rp.refuseNetwork(v.netRoots); err != nil {
		return err
	}
	if err := confineScope(v.readRoots, rp.Path); err != nil {
		return err
	}
	if confineRead(v.forbidRoots, rp.Path) {
		return &os.PathError{Op: "open", Path: rp.Path, Err: os.ErrNotExist}
	}
	return nil
}

// read opens the file inside the read boundary and reads it whole, up to the
// bound; a file past it is refused before any byte is decoded.
func (v viewImage) read(rp ResolvedPath) ([]byte, error) {
	f, err := os.Open(rp.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rp.DisplayPath, err)
	}
	defer f.Close()
	if err := confineOpened(v.readRoots, f); err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rp.DisplayPath, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not an image file", rp.DisplayPath)
	}
	limit := v.maxBytes
	if limit <= 0 {
		limit = viewImageMaxBytes
	}
	if st.Size() > limit {
		return nil, tool.Refusal{Code: CodeImageTooLarge, Message: fmt.Sprintf("%s is %d bytes; view_image reads files up to %d bytes", rp.DisplayPath, st.Size(), limit)}
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rp.DisplayPath, err)
	}
	if int64(len(raw)) > limit {
		return nil, tool.Refusal{Code: CodeImageTooLarge, Message: fmt.Sprintf("%s grew past %d bytes while it was read", rp.DisplayPath, limit)}
	}
	return raw, nil
}
