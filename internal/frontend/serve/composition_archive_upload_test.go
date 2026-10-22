package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

func TestNativeArchiveUploadAndLegacyEnvelopeKeepTheExactBinding(t *testing.T) {
	_, srv, root, _ := twoProjects(t)
	definition := agentcomposition.Builtins()[1]
	binding := &agentcomposition.Binding{
		Version:  agentcomposition.Version{Definition: definition, Revision: agentcomposition.Revision(definition)},
		Resolved: definition, Workspace: root,
	}
	binding.Seal()
	archive, err := json.Marshal(sessionstore.NativeArchive{
		Format: sessionstore.NativeArchiveFormat, Version: sessionstore.NativeArchiveVersion,
		Records: []provider.Message{{Role: provider.RoleSystem, Content: "preserved", AgentComposition: binding}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		path := "/tree/sessions/import-native?workspace=" + url.QueryEscape(root)
		content := string(archive)
		if legacy {
			path = "/tree/sessions/import-native"
			body, err := json.Marshal(map[string]any{"workspace": root, "content": json.RawMessage(archive)})
			if err != nil {
				t.Fatal(err)
			}
			content = string(body)
		}
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Path string `json:"path"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("legacy=%v: %d %v", legacy, resp.StatusCode, err)
		}
		loaded, err := sessionstore.LoadSession(result.Path)
		if err != nil || loaded.Composition() == nil || loaded.Composition().Digest != binding.Digest {
			t.Fatalf("legacy=%v lost exact binding: %v", legacy, err)
		}
	}
}

func TestNativeArchiveUploadUsesTheHostDecoderAndTypedRefusals(t *testing.T) {
	_, srv, root, _ := twoProjects(t)
	for _, tc := range []struct {
		name, data, code string
	}{
		{"valid", `{"format":"reasonix-native-session","version":2,"records":[{"role":"system","content":"retained prompt"}]}`, ""},
		{"malformed", `{broken`, "session.archive_invalid"},
		{"trailing", `{} {}`, "session.archive_invalid"},
		{"unknown version", `{"format":"reasonix-native-session","version":99,"records":[]}`, "session.archive_unsupported"},
		{"unknown field", `{"extra":true}`, "session.archive_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(namesIn(t, srv, root))
			resp, err := http.Post(srv.URL+"/tree/sessions/import-native?workspace="+url.QueryEscape(root),
				"application/json", strings.NewReader(tc.data))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if tc.code == "" {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("import = %d %v", resp.StatusCode, body)
				}
				loaded, err := sessionstore.LoadSession(body["path"].(string))
				if err != nil || !loaded.HasSystemMessage() {
					t.Fatalf("import lost history: %v", err)
				}
			} else {
				if resp.StatusCode != http.StatusBadRequest || body["code"] != tc.code {
					t.Fatalf("refusal = %d %v, want %s", resp.StatusCode, body, tc.code)
				}
				if len(namesIn(t, srv, root)) != before {
					t.Fatal("refused upload created a session")
				}
			}
		})
	}
}

func TestNativeArchiveUploadRefusesOversizeBeforeReading(t *testing.T) {
	h, _, root, _ := twoProjects(t)
	r := httptest.NewRequest(http.MethodPost, "/tree/sessions/import-native?workspace="+url.QueryEscape(root), strings.NewReader(""))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = sessionstore.MaxNativeArchiveBytes + 1
	w := httptest.NewRecorder()
	h.importNativeSession(w, r)
	var body Reason
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusRequestEntityTooLarge || body.Code != "session.archive_too_large" {
		t.Fatalf("oversize refusal = %d %+v", w.Code, body)
	}
}
