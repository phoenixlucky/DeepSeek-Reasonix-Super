package plugin

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"reasonix/internal/base/fileutil"
)

const signInConsentFile = "signin-consent.json"

type signInConsent struct {
	Version int      `json:"version"`
	Hosts   []string `json:"hosts"`
}

// AuthorizationHost is the host a browser would be sent to for this
// authorization URL, lower-cased with its port; "" when the URL names none.
func AuthorizationHost(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

func signInConsentPath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return ""
	}
	return filepath.Join(stateDir, signInConsentFile)
}

func readSignInConsent(stateDir string) signInConsent {
	var c signInConsent
	path := signInConsentPath(stateDir)
	if path == "" {
		return c
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Version != 1 {
		return signInConsent{}
	}
	return c
}

// SignInHostRemembered reports whether the user already agreed, for this
// server, to be sent to host for sign-in. The file sits beside the server's
// OAuth state, so removing that state forgets the agreement with it.
func SignInHostRemembered(stateDir, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host != "" && slices.Contains(readSignInConsent(stateDir).Hosts, host)
}

// RememberSignInHost records the user's agreement to be sent to host for this
// server's sign-in. It only ever skips the question; the browser is still sent
// to the authorization endpoint the server's metadata names.
func RememberSignInHost(stateDir, host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	path := signInConsentPath(stateDir)
	if host == "" || path == "" {
		return errors.New("MCP sign-in consent: no host or state directory to record it in")
	}
	c := readSignInConsent(stateDir)
	if slices.Contains(c.Hosts, host) {
		return nil
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	c = signInConsent{Version: 1, Hosts: append(slices.Clone(c.Hosts), host)}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, raw, 0o600)
}

// ForgetSignInConsent withdraws every remembered host of one server.
func ForgetSignInConsent(stateDir string) error {
	if strings.TrimSpace(stateDir) == "" {
		return nil
	}
	root, err := os.OpenRoot(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(signInConsentFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
