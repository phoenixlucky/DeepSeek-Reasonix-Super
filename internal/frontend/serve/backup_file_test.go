package serve

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/ext/configbackup"
)

const fileTestPassphrase = "correct horse battery"

const fileTestConfig = `
default_model = "work/model-a"

[[providers]]
name = "work"
kind = "openai"
base_url = "https://api.example.com/v1"
api_key_env = "WORK_API_KEY"
models = ["model-a"]
`

func localBackupServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := noProviderServer(t)
	if err := os.MkdirAll(filepath.Dir(config.UserConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.UserConfigPath(), []byte(fileTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORK_API_KEY", "sk-never-in-the-file-in-plain")
	srv := httptest.NewServer(operatorHandler(s))
	t.Cleanup(srv.Close)
	return srv
}

func postFile(t *testing.T, url string, body any) (int, []byte) {
	t.Helper()
	raw, _ := json.Marshal(body)
	res, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(res.Body)
	return res.StatusCode, out.Bytes()
}

type exportedFile struct {
	Name string `json:"name"`
	File string `json:"file"`
}

func exportFile(t *testing.T, srv *httptest.Server, categories []string, pass string) (int, exportedFile, []byte) {
	t.Helper()
	status, raw := postFile(t, srv.URL+"/backups/file/export", map[string]any{"categories": categories, "passphrase": pass})
	var out exportedFile
	_ = json.Unmarshal(raw, &out)
	return status, out, raw
}

// A file export needs no account: it is the machine's own configuration, sealed
// under a passphrase the person typed, and handed back to be saved by them.
func TestFileExportIsSealedAndNeedsNoAccount(t *testing.T) {
	srv := localBackupServer(t)
	status, out, raw := exportFile(t, srv, []string{"settings", "secrets"}, fileTestPassphrase)
	if status != http.StatusOK || out.File == "" || !strings.HasSuffix(out.Name, ".rxcb") {
		t.Fatalf("export = %d %s", status, raw)
	}
	envelope, err := base64.StdEncoding.DecodeString(out.File)
	if err != nil {
		t.Fatalf("the file is not base64: %v", err)
	}
	for _, leak := range []string{"api.example.com", "sk-never-in-the-file-in-plain", "model-a"} {
		if bytes.Contains(envelope, []byte(leak)) || strings.Contains(out.File, leak) {
			t.Fatalf("the file carries %q in the clear", leak)
		}
	}
	if _, err := configbackup.Open(envelope, fileTestPassphrase); err != nil {
		t.Fatalf("the person's passphrase does not open it: %v", err)
	}
}

func TestFileCatalogNeedsNoAccountEither(t *testing.T) {
	srv := localBackupServer(t)
	res, err := http.Get(srv.URL + "/backups/file")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Categories    []configbackup.CategoryInfo `json:"categories"`
		MinPassphrase int                         `json:"minPassphrase"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&body) != nil || len(body.Categories) == 0 || body.MinPassphrase != configbackup.MinPassphraseRunes {
		t.Fatalf("catalog = %d %+v", res.StatusCode, body)
	}
}

func TestFileExportRefusesAWeakPassphraseAndUnknownCategories(t *testing.T) {
	srv := localBackupServer(t)
	if status, _, raw := exportFile(t, srv, []string{"settings"}, "short"); status != http.StatusBadRequest || !strings.Contains(string(raw), "backup.weak_passphrase") {
		t.Fatalf("weak passphrase = %d %s", status, raw)
	}
	if status, _, raw := exportFile(t, srv, []string{"nonsense"}, fileTestPassphrase); status != http.StatusBadRequest || !strings.Contains(string(raw), "backup.bad_categories") {
		t.Fatalf("unknown category = %d %s", status, raw)
	}
	if status, _, raw := exportFile(t, srv, nil, fileTestPassphrase); status != http.StatusBadRequest || !strings.Contains(string(raw), "backup.bad_categories") {
		t.Fatalf("no category = %d %s", status, raw)
	}
}

func TestFileImportPreviewsWithTheRightPassphraseOnly(t *testing.T) {
	srv := localBackupServer(t)
	_, out, _ := exportFile(t, srv, []string{"settings"}, fileTestPassphrase)

	status, raw := postFile(t, srv.URL+"/backups/file/preview", map[string]any{"file": out.File, "passphrase": "wrong passphrase!"})
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "backup.cannot_decrypt") {
		t.Fatalf("wrong passphrase = %d %s", status, raw)
	}
	status, raw = postFile(t, srv.URL+"/backups/file/preview", map[string]any{"file": "!!not base64!!", "passphrase": fileTestPassphrase})
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "backup.malformed") {
		t.Fatalf("not a file = %d %s", status, raw)
	}
	status, raw = postFile(t, srv.URL+"/backups/file/preview", map[string]any{"file": out.File, "passphrase": fileTestPassphrase})
	var plan configbackup.Plan
	if status != http.StatusOK || json.Unmarshal(raw, &plan) != nil || plan.ID == "" || len(plan.Items) == 0 {
		t.Fatalf("preview = %d %s", status, raw)
	}

	status, raw = postFile(t, srv.URL+"/backups/file/apply", map[string]any{"planId": plan.ID, "items": []string{plan.Items[0].ID}})
	if status != http.StatusOK || !strings.Contains(string(raw), `"applied"`) {
		t.Fatalf("apply = %d %s", status, raw)
	}
	status, raw = postFile(t, srv.URL+"/backups/file/apply", map[string]any{"planId": "never-issued", "items": []string{"x"}})
	if status != http.StatusGone || !strings.Contains(string(raw), "backup.plan_expired") {
		t.Fatalf("unknown plan = %d %s", status, raw)
	}
}

// What a window holds, a paired device does not: the file holds keys, and a
// phone on the relay must never be the one to ask for it or to write one in.
func TestFileRoutesAreRefusedToAPairedDevice(t *testing.T) {
	s := noProviderServer(t)
	h := s.Handler()
	for _, tc := range []struct{ path, body string }{
		{"/backups/file/export", `{"categories":["settings"],"passphrase":"correct horse battery"}`},
		{"/backups/file/preview", `{"file":"AAAA","passphrase":"correct horse battery"}`},
		{"/backups/file/apply", `{"planId":"x","items":["y"]}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+s.AuthToken())
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(withDeviceReach(context.Background(), "phone-1", 1))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), codeDeviceHostOnly) {
			t.Fatalf("%s through a device = %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestFileImportRefusesAnOversizedBody(t *testing.T) {
	srv := localBackupServer(t)
	huge := strings.Repeat("A", configbackup.MaxEnvelopeBytes*2)
	status, raw := postFile(t, srv.URL+"/backups/file/preview", map[string]any{"file": huge, "passphrase": fileTestPassphrase})
	if status != http.StatusRequestEntityTooLarge || !strings.Contains(string(raw), "backup.too_large") {
		t.Fatalf("oversized = %d %.200s", status, raw)
	}
}
