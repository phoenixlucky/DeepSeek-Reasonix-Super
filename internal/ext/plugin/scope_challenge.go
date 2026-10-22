package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/base/fileutil"
	"reasonix/internal/base/textutil"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
)

// Step-up authorization (MCP 2025-11-25, Scope Challenge Handling): a 403 whose
// Bearer error is insufficient_scope names the scope the operation needs. That
// header is the server's text: shown and passed on, never used to decide.

const (
	maxChallengeScopes = 32
	maxScopeLength     = 128
	// maxScopeUpgrades bounds how many times one operation may ask for more
	// access before it is treated as a permanent failure.
	maxScopeUpgrades   = 2
	scopeChallengeFile = "mcp-oauth-scope-challenge.json"
	// scopeUpgradeMemory is how long a count of upgrades for an operation lasts.
	scopeUpgradeMemory = 24 * time.Hour
)

// ErrNoScopeUpgradePending is a request to extend access when no server asked.
var ErrNoScopeUpgradePending = errors.New("no MCP server asked for more access")

// ErrInsufficientScope is the identity of a 403 insufficient_scope answer from a
// server whose access the user granted by signing in.
var ErrInsufficientScope = errors.New("MCP server needs more access than the user granted")

// ErrScopeUpgradeExhausted is the same answer after asking for more access has
// not helped: the server keeps naming scopes that were already requested, or the
// operation has used its upgrades. Retrying cannot succeed.
var ErrScopeUpgradeExhausted = errors.New("MCP server still refuses after the access was extended")

// InsufficientScopeError carries what the server asked for.
type InsufficientScopeError struct {
	Server    string
	Resource  string
	Scopes    []string
	Exhausted bool
	cause     *httpStatusError
}

func (e *InsufficientScopeError) Error() string {
	return fmt.Sprintf("MCP server %q needs more access than the user granted: %v", e.Server, e.cause)
}

func (e *InsufficientScopeError) Unwrap() []error {
	if e.Exhausted {
		return []error{ErrScopeUpgradeExhausted, ErrInsufficientScope, e.cause}
	}
	return []error{ErrInsufficientScope, e.cause}
}

func (e *InsufficientScopeError) DiagnosticFacts() string { return e.Error() }

// bearerChallenge is what a Bearer WWW-Authenticate header carries.
type bearerChallenge struct {
	MetadataURL string
	Scope       string
	Error       string
}

// scopeTokens reads the scope-token list of an RFC 6749 scope value: space
// separated tokens of printable characters other than quote and backslash. A
// token that is not one, an over-long one and a repeat are dropped, and the list
// is capped, because the header is a third party's.
func scopeTokens(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for tok := range strings.FieldsSeq(raw) {
		if len(tok) > maxScopeLength || seen[tok] {
			continue
		}
		valid := true
		for i := range len(tok) {
			if c := tok[i]; c < 0x21 || c > 0x7e || c == '"' || c == '\\' {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		seen[tok] = true
		if out = append(out, tok); len(out) == maxChallengeScopes {
			break
		}
	}
	return out
}

// scopeChallenge is a pending request for more access, kept beside the server's
// own OAuth state so any frontend, and the next process, finds the same one.
type scopeChallenge struct {
	Resource string    `json:"resource"`
	Scopes   []string  `json:"scopes"`
	At       time.Time `json:"at"`
	// Upgrades counts how many times this operation's challenge was answered.
	Upgrades map[string]int `json:"upgrades,omitempty"`
}

func scopeChallengePath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return ""
	}
	return filepath.Join(stateDir, scopeChallengeFile)
}

func loadScopeChallenge(stateDir string) (scopeChallenge, bool) {
	path := scopeChallengePath(stateDir)
	if path == "" {
		return scopeChallenge{}, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxOAuthBody {
		return scopeChallenge{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return scopeChallenge{}, false
	}
	var c scopeChallenge
	if json.Unmarshal(b, &c) != nil || len(c.Scopes) > maxChallengeScopes {
		return scopeChallenge{}, false
	}
	return c, true
}

func saveScopeChallenge(stateDir string, c scopeChallenge) error {
	path := scopeChallengePath(stateDir)
	if path == "" {
		return errors.New("private state directory is unavailable")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFileStrict(path, append(b, '\n'), 0o600)
}

// PendingScopeUpgrade lists the scopes a server asked for that the user has not
// yet been asked to grant, or none.
func PendingScopeUpgrade(stateDir string) []string {
	c, ok := loadScopeChallenge(stateDir)
	if !ok {
		return nil
	}
	return append([]string(nil), c.Scopes...)
}

// ClearScopeUpgrade forgets the pending request once the upgrade is granted. The
// per-operation counts stay, so a server that keeps asking is still stopped.
func ClearScopeUpgrade(stateDir string) {
	if c, ok := loadScopeChallenge(stateDir); ok {
		c.Scopes = nil
		_ = saveScopeChallenge(stateDir, c)
	}
}

// mergeScopes is the access to ask for when stepping up: what was granted, then
// what is newly asked for. Dropping a granted scope would make the upgrade a
// downgrade for everything else the server was already used for.
func mergeScopes(granted string, asked ...[]string) string {
	out := scopeTokens(granted)
	seen := map[string]bool{}
	for _, s := range out {
		seen[s] = true
	}
	for _, list := range asked {
		for _, s := range list {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return strings.Join(out, " ")
}

// allGranted says every scope a challenge names was already part of the grant.
func allGranted(granted string, scopes []string) bool {
	have := map[string]bool{}
	for _, s := range scopeTokens(granted) {
		have[s] = true
	}
	for _, s := range scopes {
		if !have[s] {
			return false
		}
	}
	return len(scopes) > 0
}

// recordInsufficientScope notes a challenge and decides whether the operation has
// used up its upgrades. An operation whose challenge names only scopes already
// granted, or that has asked twice, will not be helped by asking again.
func recordInsufficientScope(stateDir, resource, operation string, scopes []string) (exhausted bool) {
	state, _ := loadMCPOAuthState(stateDir)
	c, _ := loadScopeChallenge(stateDir)
	if time.Since(c.At) > scopeUpgradeMemory {
		c.Upgrades = nil
	}
	if c.Upgrades == nil {
		c.Upgrades = map[string]int{}
	}
	exhausted = allGranted(state.Scope, scopes) || c.Upgrades[operation] >= maxScopeUpgrades
	if !exhausted {
		c.Upgrades[operation]++
	}
	c.Resource, c.Scopes, c.At = resource, scopes, time.Now()
	_ = saveScopeChallenge(stateDir, c)
	return exhausted
}

// scopeRefusal turns an insufficient_scope answer into the refusal a model reads
// and tells the user, once, that this server needs more access. Any other error
// passes through unchanged.
func (c *Client) scopeRefusal(err error) error {
	var need *InsufficientScopeError
	if !errors.As(err, &need) {
		return err
	}
	c.announceScopeChallenge(need)
	if need.Exhausted {
		return tool.Refusal{Code: tool.CodeMCPScopeUpgradeExhausted,
			Message: fmt.Sprintf("MCP server %q still refuses this call after the user extended its access. Do not retry it; tell the user what you were trying to do.", c.name)}
	}
	return tool.Refusal{Code: tool.CodeMCPInsufficientScope,
		Message: fmt.Sprintf("MCP server %q refused this call: the access the user granted it does not cover this operation. Only the user can authorize more, and they have been told. Say what you were trying to do and wait; do not retry until they say it is done.", c.name)}
}

func (c *Client) announceScopeChallenge(need *InsufficientScopeError) {
	h := c.owner.Load()
	asked := strings.Join(need.Scopes, " ")
	if h == nil || need.Exhausted || c.scopeAnnounced.Swap(asked) == asked {
		return
	}
	h.statusMu.RLock()
	sink := h.statusSink
	h.statusMu.RUnlock()
	if sink == nil {
		return
	}
	sink.Emit(event.Event{
		Kind: event.Notice, Audience: event.NoticeAudienceOperator, Level: event.LevelWarn,
		Code:   event.NoticeCodeMCPScopeUpgrade,
		Text:   fmt.Sprintf("MCP server %q asks for more access than you granted it. Sign in again to review and grant it.", textutil.ShownLocator(c.spec.Name)),
		Detail: c.spec.Name,
	})
}

// authorizationScope is the scope an authorization request asks for: the
// server's challenge or its supported list, and for a step-up the granted scope
// merged with what the server asked for.
func authorizationScope(spec Spec, challengedScope string, resourceMeta protectedResourceMetadata, resource string) (string, error) {
	scope := strings.TrimSpace(challengedScope)
	if scope == "" {
		scope = strings.Join(resourceMeta.ScopesSupported, " ")
	}
	if !spec.OAuthScopeUpgrade {
		return scope, nil
	}
	asked := PendingScopeUpgrade(spec.StateDir)
	if len(asked) == 0 {
		return "", ErrNoScopeUpgradePending
	}
	granted := ""
	if prior, loadErr := loadMCPOAuthState(spec.StateDir); loadErr == nil && sameCanonicalResource(prior.Resource, resource) {
		granted = prior.Scope
	}
	return mergeScopes(granted, asked), nil
}
