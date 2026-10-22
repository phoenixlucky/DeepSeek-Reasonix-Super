package control

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
)

// paidSearchApprover asks the user before web_search uses a backend the vendor
// charges for. Only a person agrees to be charged, so no approval mode answers
// it; a session grant covers one backend, and "always allow" records the backend
// in the user's own [tools] web_search_paid_allowed.
type paidSearchApprover struct{ c *Controller }

func (p paidSearchApprover) ApprovePaidSearch(ctx context.Context, req tool.PaidSearchRequest) (bool, error) {
	fetch := req.Service == tool.PaidServiceFetch
	subject, reasonFmt := req.Backend, i18n.M.PaidSearchApprovalReasonFmt
	if fetch {
		subject, reasonFmt = req.Backend+"/fetch", i18n.M.PaidFetchApprovalReasonFmt
	}
	if p.c.approval.preApprovedForDecision(PaidSearchApprovalTool, subject, nil, true) {
		return true, nil
	}
	args, _ := json.Marshal(map[string]string{"backend": req.Backend, "service": req.Service, "query": req.Query})
	reply, err := p.c.requestApprovalDecision(ctx, approvalRequest{tool: PaidSearchApprovalTool, subject: subject, args: args,
		reason: fmt.Sprintf(reasonFmt, req.Backend, req.Price, req.Query), fresh: true})
	if err != nil || !reply.allow {
		return false, err
	}
	if reply.session || reply.persist {
		p.c.approval.grantSession(PaidSearchApprovalTool, subject)
	}
	if reply.persist {
		p.c.emitRememberResult(rememberPaidBackend(req.Backend, fetch))
	}
	return true, nil
}

// rememberPaidBackend records the user's agreement to be charged by one backend
// for searching or for fetching. A backend already recorded is not written again.
func rememberPaidBackend(backend string, fetch bool) RememberResult {
	backend = strings.TrimSpace(backend)
	key := "web_search_paid_allowed"
	if fetch {
		key = "web_fetch_paid_allowed"
	}
	rule := "tools." + key + " += " + backend
	if backend == "" {
		return RememberResult{Rule: rule, Err: fmt.Errorf("no backend to allow")}
	}
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	list := &cfg.Tools.WebSearchPaidAllowed
	if fetch {
		list = &cfg.Tools.WebFetchPaidAllowed
	}
	if slices.Contains(*list, backend) {
		return RememberResult{Rule: rule, Path: path, CoveredBy: backend}
	}
	*list = append(slices.Clone(*list), backend)
	if err := cfg.SaveTo(path); err != nil {
		return RememberResult{Rule: rule, Path: path, Err: err}
	}
	return RememberResult{Rule: rule, Path: path, Saved: true}
}
