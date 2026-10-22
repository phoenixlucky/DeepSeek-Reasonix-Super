package boot

import (
	"slices"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/ext/modhost"
	"reasonix/internal/session/control"
)

// Until the switch is available a mod never runs, whatever the user file says,
// and its status names why.
func TestEffectAModStaysOffWhileTheSwitchIsWithheldEvenIfTheFileSaysOn(t *testing.T) {
	_, _, prov := installMod(t, denyingMod)
	modhost.Complete = false
	setModsSwitch(t, true)

	runModTurn(t, prov, "echo FORBIDDEN-ran")
	got := strings.Join(prov.toolResults(), "\n")
	if strings.Contains(got, "mod says") || !strings.Contains(got, "FORBIDDEN-ran") {
		t.Fatalf("a withheld mod still ruled on the call:\n%s", got)
	}
	if st := modhost.Default.Snapshot(); len(st) != 1 || st[0].State != modhost.StateInactive || st[0].Code != modhost.CodeWithheld {
		t.Fatalf("status = %+v, want inactive with %s", st, modhost.CodeWithheld)
	}
}

func receiptKinds(t *testing.T, prov *modProvider, command string) []string {
	t.Helper()
	prov.command = command
	done := make(chan []string, 4)
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind != event.TurnDone {
			return
		}
		var kinds []string
		if e.Receipt != nil {
			for _, g := range e.Receipt.Gaps {
				kinds = append(kinds, g.Kind)
			}
		}
		done <- kinds
	})
	res, err := BuildRuntime(t.Context(), Options{Sink: sink})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Controller.Close()
	res.Controller.SetToolApprovalMode(control.ToolApprovalYolo)
	res.Controller.Submit("run it")
	select {
	case kinds := <-done:
		return kinds
	case <-time.After(30 * time.Second):
		t.Fatal("the turn never reported TurnDone")
		return nil
	}
}

// The turn's receipt, as the frontend gets it through the real assembly, says a
// mod's effects were not observed — only while a mod is running.
func TestEffectTheReceiptNamesModEffectsUnobservedOnlyWhileAModRuns(t *testing.T) {
	_, _, prov := installMod(t, denyingMod)

	setModsSwitch(t, true)
	if kinds := receiptKinds(t, prov, "echo hi"); !slices.Contains(kinds, "mod_effects_unobserved") {
		t.Fatalf("receipt gaps with a mod running = %v", kinds)
	}

	resetProvider(prov)
	setModsSwitch(t, false)
	if kinds := receiptKinds(t, prov, "echo hi"); slices.Contains(kinds, "mod_effects_unobserved") {
		t.Fatalf("receipt gaps with the switch off = %v", kinds)
	}
}
