package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/safety/permission"
	"reasonix/internal/state/sessionstore"
)

// noteOriginMatch counts a call that is not read-only and whose arguments carry,
// verbatim, text an external result gave this task. Nothing is decided on it: the
// number goes to the turn's audit, so a later gate can be set from measured rates.
func (a *Agent) noteOriginMatch(plan *toolCallPlan) {
	if plan == nil || plan.readOnly || a.task.origins == nil {
		return
	}
	if len(a.task.origins.match(plan.permArgs, a.userTexts)) > 0 {
		a.task.origins.noteMatch()
	}
}

// userTexts is what the user wrote in this session, for telling a string they
// typed from one they only saw.
func (a *Agent) userTexts() []string {
	if a.sess.conversation == nil {
		return nil
	}
	var out []string
	for _, m := range a.sess.conversation.Snapshot() {
		if m.Role == provider.RoleUser {
			out = append(out, sessionstore.UserMessageText(m))
		}
	}
	return out
}

// originCtx marks ctx with the first origin whose recorded text the call's
// arguments repeat verbatim, for the permission gate to ask about when the user
// has switched that on. It marks and decides nothing itself.
func (a *Agent) originCtx(ctx context.Context, plan *toolCallPlan) context.Context {
	if plan == nil || plan.readOnly || a.task.origins == nil {
		return ctx
	}
	origins := a.task.origins.match(plan.permArgs, a.userTexts)
	if len(origins) == 0 {
		return ctx
	}
	return permission.WithOrigin(ctx, originLabels(origins))
}

// originLabels names every origin the call repeats, at most three, then how many more.
func originLabels(origins []tool.Provenance) string {
	const shown = 3
	var names []string
	for _, o := range origins {
		if l := tool.ProvenanceLabel(o); !slices.Contains(names, l) {
			names = append(names, l)
		}
	}
	if len(names) <= shown {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:shown], ", "), len(names)-shown)
}
