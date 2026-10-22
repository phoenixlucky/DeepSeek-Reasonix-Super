package remotehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/frontend/serve"
	"reasonix/internal/platform/remote"
	"reasonix/internal/platform/remote/attach"
)

func TestWSLRequestsNeverReachTheSSHTransport(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	entry := config.RemoteHostEntry{Name: "dev", Kind: config.RemoteKindWSL, Distro: "Ubuntu", Workspace: `C:\work\project`}
	if err := config.EditUserConfigWithCredentials(func(c *config.Config) ([]config.CredentialChange, error) {
		return nil, c.UpsertRemoteHost(entry)
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if client, err := attach.Dial(cfg, entry.Name, attach.Prompts{}); !errors.Is(err, remote.ErrNotSSHHost) || client != nil {
		t.Fatalf("SSH construction = %v, %v; want no client and ErrNotSSHHost", client, err)
	}
	for _, route := range []string{"probe", "dirs", "open", "tree", "sessions/pin", "sessions/archive", "sessions/remove"} {
		t.Run(route, func(t *testing.T) {
			sshCalls, wslCalls := 0, 0
			link := &Link{pool: attach.NewPool(t.Context(), attach.Options{
				Dial: func(string, attach.Prompts) (*remote.Client, error) {
					sshCalls++
					return nil, fmt.Errorf("unexpected SSH construction")
				},
				DialWSL: func(got config.RemoteHostEntry) (attach.Link, error) {
					wslCalls++
					if got.Name != entry.Name || got.Distro != entry.Distro || got.Host != "" {
						t.Errorf("WSL entry = %+v", got)
					}
					return nil, fmt.Errorf("WSL fixture unavailable")
				},
			})}
			hub := serve.NewHub(serve.HubOptions{Remote: link})
			method, path, body := http.MethodGet, "/remotes/dev/"+route, ""
			if route == "open" {
				method, path, body = http.MethodPost, "/remotes/open", `{"host":"dev"}`
			} else if strings.HasPrefix(route, "sessions/") {
				method, body = http.MethodPost, `{"path":"/session.jsonl","pinned":true,"archived":true}`
			}
			req := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+hub.AuthToken())
			req.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			hub.Handler().ServeHTTP(out, req)
			var reason serve.Reason
			if err := json.Unmarshal(out.Body.Bytes(), &reason); err != nil {
				t.Fatal(err)
			}
			if out.Code != http.StatusBadGateway || reason.Code != "remote.attach_failed" || !strings.Contains(out.Body.String(), "WSL fixture unavailable") || sshCalls != 0 || wslCalls != 1 {
				t.Fatalf("status=%d body=%s SSH=%d WSL=%d", out.Code, out.Body.String(), sshCalls, wslCalls)
			}
		})
	}
}
