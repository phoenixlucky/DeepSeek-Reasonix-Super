package control

import (
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/safety/egress"
)

// egressPersistable reports whether "always allow this host" has somewhere to
// go: the user's own [sandbox] allowed_domains list. When the only list in
// force is a project's, writing a host to the user file would turn an absent
// user list into a one-host restriction that displaces the project's, so the
// choice is not offered.
func (c *Controller) egressPersistable() bool {
	return len(config.LoadForEdit(config.UserConfigPath()).Sandbox.AllowedDomains) > 0
}

// rememberEgressHost adds one host to the user's allowed_domains. A host the
// list already names, or that the deny list outranks, is not written.
func (c *Controller) rememberEgressHost(host string) RememberResult {
	host = strings.ToLower(strings.TrimSpace(host))
	rule := "sandbox.allowed_domains += " + host
	if host == "" {
		return RememberResult{Rule: rule, Err: fmt.Errorf("no host to allow")}
	}
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	if len(cfg.Sandbox.AllowedDomains) == 0 {
		return RememberResult{Rule: rule, Path: path, Err: fmt.Errorf("the user config has no allowed_domains list to add to")}
	}
	policy := egress.Policy{Allow: cfg.Sandbox.AllowedDomains, Deny: cfg.Sandbox.DeniedDomains}
	switch policy.Decide(host) {
	case "":
		return RememberResult{Rule: rule, Path: path, CoveredBy: host}
	case egress.ReasonDenied:
		return RememberResult{Rule: rule, Path: path, Err: fmt.Errorf("%s is on your denied_domains list, which outranks allowed_domains", host)}
	}
	cfg.Sandbox.AllowedDomains = append(slices.Clone(cfg.Sandbox.AllowedDomains), host)
	if err := cfg.SaveTo(path); err != nil {
		return RememberResult{Rule: rule, Path: path, Err: err}
	}
	return RememberResult{Rule: rule, Path: path, Saved: true}
}

// offersPersist is whether a card may carry "always allow". A rule-backed tool
// needs the rule store; a host for the egress list needs a user list to join.
func (c *Controller) offersPersist(tool string, persist bool) bool {
	if tool == NetworkEgressApprovalTool {
		return c.egressPersistable()
	}
	if tool == PaidSearchApprovalTool || tool == MCPSignInApprovalTool {
		return true
	}
	return persist && c.onRemember != nil
}

// egressUnlistedAllowed answers for a host the allow list does not name when the
// user's own file says not to ask. The first time a host passes this way it is
// said once; the proxy's denials and address checks have already run or still
// follow, so only the question is skipped.
func (c *Controller) egressUnlistedAllowed(host string) bool {
	if !config.LoadForEdit(config.UserConfigPath()).EgressUnlistedAllowed() {
		return false
	}
	if !c.approval.preApprovedForDecision(NetworkEgressApprovalTool, host, nil, true) {
		c.approval.grantSession(NetworkEgressApprovalTool, host)
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeEgressAutoAllowed,
			Text: fmt.Sprintf(i18n.M.EgressAutoAllowedFmt, host), Detail: host})
	}
	return true
}
