package agent

import (
	"slices"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/extension"
	"reasonix/internal/ext/extension/dispatch"
	"reasonix/internal/runtime/completion"
	"reasonix/internal/state/sessionstore"
)

func modAgent(t *testing.T, point extension.InterceptorPoint, mods map[string]bool) *Agent {
	t.Helper()
	sidecar := extension.Contribution{Kind: extension.KindInterceptor, ID: "x", Source: extension.ContributionSource{Scope: extension.ScopePlugin, PluginID: "p"}}
	d := dispatch.New(map[extension.InterceptorPoint][]extension.Contribution{point: {sidecar}}, nil,
		func(string) dispatch.Client { return nil }, nil, dispatch.Options{Mods: mods})
	return New(nil, tool.NewRegistry(), sessionstore.NewSession(""), Options{Extensions: d, CheckTargetAccess: allowTestFileAccess}, event.Discard)
}

// A mod is a full-trust process with no sandbox: where it subscribes does not
// bound what it can write, so a live mod is an unseen writer on any point.
func TestALiveModIsAnUnseenWriterWhereverItSubscribes(t *testing.T) {
	for _, point := range []extension.InterceptorPoint{extension.PointInputReceive, extension.PointSessionStart, extension.PointToolBefore} {
		a := modAgent(t, point, map[string]bool{"p": true})
		if !a.unseenToolWriter() || !a.checkContract().UnseenWriter() {
			t.Errorf("a mod on %s is not an unseen writer", point)
		}
	}
	if a := modAgent(t, extension.PointInputReceive, nil); a.unseenToolWriter() {
		t.Error("an ordinary sidecar on input.receive became an unseen writer; that boundary is pinned elsewhere")
	}
}

func modGaps(r *event.CompletionReceipt) []string {
	var out []string
	if r != nil {
		for _, g := range r.Gaps {
			if g.Kind == completion.GapModEffectsUnobserved.String() {
				out = append(out, g.Kind)
			}
		}
	}
	return out
}

// The receipt says so whenever a mod was live, even for a turn that had nothing
// else to judge: the claim is about what the host can establish, not about what
// the mod did.
func TestTheReceiptNamesModEffectsUnobservedOnlyWhileAModIsLive(t *testing.T) {
	withMod := modAgent(t, extension.PointInputReceive, map[string]bool{"p": true})
	got := withMod.CompletionReceipt()
	if got == nil || !slices.Equal(modGaps(got), []string{"mod_effects_unobserved"}) || !got.SaysSomething() || got.Severity() == event.ReceiptSeverityNone {
		t.Fatalf("receipt with a live mod and no report = %+v", got)
	}

	withMod.turn.completion = &completion.Report{Verdict: completion.VerdictDone}
	if got := withMod.CompletionReceipt(); len(modGaps(got)) != 1 || got.Verdict != "done" {
		t.Fatalf("receipt with a live mod and a report = %+v, want the report's verdict plus one mod gap", got)
	}

	without := modAgent(t, extension.PointInputReceive, nil)
	if got := without.CompletionReceipt(); got != nil {
		t.Fatalf("receipt with no mod and no report = %+v, want nil", got)
	}
	without.turn.completion = &completion.Report{Verdict: completion.VerdictDone}
	if got := without.CompletionReceipt(); len(modGaps(got)) != 0 {
		t.Fatalf("receipt without a mod carries the mod gap: %+v", got)
	}
}
