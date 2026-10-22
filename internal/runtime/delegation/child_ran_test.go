package delegation

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
)

func (s *loneSink) ranOn() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ran...)
}

// What a person asks is "was the wrong model used", so the fact the window shows is
// the model and effort the child started with. Every status event for the child
// carries it, from running to the terminal one, so an interrupted run has it too.
func TestEachStatusOfAChildNamesTheModelItRanOn(t *testing.T) {
	for _, tc := range []struct {
		name, args, want string
	}{
		{"inherits the parent's", `{"prompt":"work","description":"lone"}`, "base/high"},
		{"names its own", `{"prompt":"work","description":"lone","model":"other/fast","effort":"low"}`, "other/fast/low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov := newLoneProvider()
			close(prov.release)
			task, ctx, _, sink := loneFixture(t, prov, 4)
			if _, err := task.Execute(ctx, json.RawMessage(tc.args)); err != nil {
				t.Fatalf("task: %v", err)
			}
			got := sink.ranOn()
			if len(got) < 2 {
				t.Fatalf("status events naming the model = %v, want at least running and the terminal one", got)
			}
			for _, line := range got {
				if _, model, _ := strings.Cut(line, "="); model != tc.want {
					t.Errorf("status %q names %q, want %q", line, model, tc.want)
				}
			}
			if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "completed=") }) {
				t.Errorf("the terminal status does not carry it: %v", got)
			}
		})
	}
}

// A batch's items may each pick a model, so the statuses are per child: two items
// that chose differently each carry their own, and the batch itself carries none.
func TestEachItemOfABatchNamesItsOwnModel(t *testing.T) {
	task := newTestTaskTool(t, parallelStaticProvider{}, tool.NewRegistry(), "sys", "", "", nil)
	parallel := NewParallelTasksTool(task, tool.NewRegistry())
	var mu sync.Mutex
	ran := map[string]map[string]bool{}
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind != event.ToolProgress || e.Tool.Name != event.SubagentProgressStatusName || e.Tool.Profile == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if ran[e.Tool.ID] == nil {
			ran[e.Tool.ID] = map[string]bool{}
		}
		ran[e.Tool.ID][e.Tool.Profile.Model+"/"+e.Tool.Profile.Effort] = true
	})
	ctx := agent.WithCallContext(context.Background(), "batch-call", sink, nil, false)
	if _, err := parallel.Execute(ctx, json.RawMessage(`{"tasks":[{"prompt":"first","model":"m/one","effort":"low"},{"prompt":"second"}]}`)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	var got []string
	for id, set := range ran {
		if len(set) != 1 {
			t.Errorf("child %s was said to run on %v, want exactly one identity", id, set)
		}
		for m := range set {
			got = append(got, m)
		}
	}
	slices.Sort(got)
	if want := []string{"base-model/base-effort", "m/one/low"}; !slices.Equal(got, want) {
		t.Fatalf("children ran on %v, want %v", got, want)
	}
}
