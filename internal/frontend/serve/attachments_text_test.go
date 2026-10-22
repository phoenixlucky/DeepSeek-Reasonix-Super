package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func postText(t *testing.T, base string, body map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/attachments", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// A paste kept as a file comes back as a pointer for the message, never as an
// "@path" token: the turn parser expands that token into the file's head, which
// would put the text straight back.
func TestTextAttachmentReturnsAPointerNotAnAtReference(t *testing.T) {
	dir := testenv.TempDir(t)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: dir})
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	defer srv.Close()

	text := strings.Repeat("a long pasted log line\n", 300)
	resp, got := postText(t, srv.URL, map[string]any{"kind": "text", "text": text})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %v", resp.StatusCode, got)
	}
	path, _ := got["path"].(string)
	pointer, _ := got["pointer"].(string)
	if !strings.HasSuffix(path, ".txt") || !strings.Contains(pointer, path) || strings.Contains(pointer, "pasted log line") {
		t.Fatalf("path %q, pointer %q", path, pointer)
	}
	if _, has := got["ref"]; has {
		t.Fatalf("a kept paste came back with an @ reference: %v", got)
	}
	if stored, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path))); err != nil || string(stored) != text {
		t.Fatalf("stored text differs (err %v)", err)
	}
}

func TestTextAttachmentRefusalsCarryTheirIdentity(t *testing.T) {
	dir := testenv.TempDir(t)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: dir})
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	defer srv.Close()

	for name, tc := range map[string]struct {
		text   string
		status int
		code   string
	}{
		"empty":        {"", http.StatusBadRequest, "attachment.empty"},
		"binary (NUL)": {"a\x00b", http.StatusUnsupportedMediaType, "attachment.not_text"},
		"over the cap": {strings.Repeat("x", control.MaxTextAttachmentBytes+1), http.StatusRequestEntityTooLarge, "attachment.too_large"},
	} {
		resp, got := postText(t, srv.URL, map[string]any{"kind": "text", "text": tc.text})
		if resp.StatusCode != tc.status || got["code"] != tc.code {
			t.Fatalf("%s: status %d code %v, want %d %s", name, resp.StatusCode, got["code"], tc.status, tc.code)
		}
	}
}
