package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/openwith"
)

type openCall struct {
	id, dir   string
	forbidden []string
}

func stubOpenWith(t *testing.T, err error) *[]openCall {
	t.Helper()
	var calls []openCall
	list, open := openWithList, openWithOpen
	openWithList = func(forbidden []string) []openwith.Status {
		return []openwith.Status{{ID: "vscode", Name: "Visual Studio Code", Available: true}, {ID: "cursor", Name: "Cursor"}}
	}
	openWithOpen = func(id, dir string, forbidden []string) (string, error) {
		calls = append(calls, openCall{id, dir, forbidden})
		return "Visual Studio Code", err
	}
	t.Cleanup(func() { openWithList, openWithOpen = list, open })
	return &calls
}

func postOpenWith(h *Hub, body string, ctx context.Context) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/host/open-with", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.openWithProject(rec, req)
	return rec
}

func TestOpenWithStartsOnlyAListedProject(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	listed, other := testenv.TempDir(t), testenv.TempDir(t)
	rememberWorkspace(listed)
	calls := stubOpenWith(t, nil)
	h := NewHub(HubOptions{})

	rec := postOpenWith(h, `{"id":"vscode","root":`+jsonString(listed)+`}`, context.Background())
	abs, _ := filepath.Abs(listed)
	if rec.Code != http.StatusOK || len(*calls) != 1 || (*calls)[0].id != "vscode" || (*calls)[0].dir != abs {
		t.Fatalf("a listed project = %d %s %+v", rec.Code, rec.Body, *calls)
	}
	if !reflect.DeepEqual((*calls)[0].forbidden, LaunchWorkspaces()) {
		t.Fatalf("a program inside the listed projects must be refused, forbidden = %v", (*calls)[0].forbidden)
	}

	*calls = nil
	rec = postOpenWith(h, `{"id":"vscode","root":`+jsonString(other)+`}`, context.Background())
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), codeWorkspaceUnknown) || len(*calls) != 0 {
		t.Fatalf("an unlisted folder = %d %s %+v", rec.Code, rec.Body, *calls)
	}
}

func TestOpenWithAnswersEachFailureWithItsOwnCode(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	listed := testenv.TempDir(t)
	rememberWorkspace(listed)
	h := NewHub(HubOptions{})
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{openwith.ErrUnknown, http.StatusNotFound, codeOpenWithUnknown},
		{openwith.ErrUnavailable, http.StatusNotFound, codeOpenWithUnavailable},
	} {
		stubOpenWith(t, tc.err)
		rec := postOpenWith(h, `{"id":"x","root":`+jsonString(listed)+`}`, context.Background())
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.code) {
			t.Fatalf("%v = %d %s, want %d %s", tc.err, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	stubOpenWith(t, context.DeadlineExceeded)
	rec := postOpenWith(h, `{"id":"x","root":`+jsonString(listed)+`}`, context.Background())
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), codeOpenWithFailed) {
		t.Fatalf("a failed start = %d %s", rec.Code, rec.Body)
	}
	if rec := postOpenWith(h, `{"root":"x"}`, context.Background()); rec.Code != http.StatusBadRequest {
		t.Fatalf("no id = %d", rec.Code)
	}
}

// What starts programs on the machine is the window's: a paired device, or a
// page that arrives by the relay, is turned away before the handler runs.
func TestOpenWithIsRefusedToAPairedDevice(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	listed := testenv.TempDir(t)
	rememberWorkspace(listed)
	calls := stubOpenWith(t, nil)
	h := NewHub(HubOptions{})
	for _, tc := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPost, `{"id":"vscode","root":` + jsonString(listed) + `}`},
	} {
		req := httptest.NewRequest(tc.method, "/host/open-with", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+h.AuthToken())
		req = req.WithContext(withDeviceReach(context.Background(), "phone-1", 1))
		rec := httptest.NewRecorder()
		h.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), codeDeviceHostOnly) {
			t.Fatalf("%s through a device = %d %s", tc.method, rec.Code, rec.Body)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("a device started %+v", *calls)
	}
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}
