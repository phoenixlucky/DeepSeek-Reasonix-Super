package agent

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/usecap"
	"reasonix/internal/state/sessionstore"
)

type argumentDecisionTool struct{ argumentProbeTool }

func (*argumentDecisionTool) ReadOnly() bool { return true }
func (*argumentDecisionTool) DecisionBarrierCall(args json.RawMessage) bool {
	var in struct {
		Op string `json:"op"`
	}
	return json.Unmarshal(args, &in) == nil && in.Op == "propose"
}

func TestArgumentNormalizationDecisionBarriers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    string
		barrier bool
	}{
		{"conditional", `{"op":"propose"}`, true},
		{"nondecision", `{"op":"list"}`, false},
		{"ask", `{"questions":[{"header":"Choice","question":"Which?","options":[{"label":"A"},{"label":"B"}]}]}`, true},
		{"proxy_ask", `{"questions":[{"header":"Choice","question":"Which?","options":[{"label":"A"},{"label":"B"}]}]}`, true},
	} {
		for _, format := range []string{"plain", "object", "string"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				reg, later := argumentProbeRegistry()
				target := tool.Tool(&argumentDecisionTool{argumentProbeTool{
					name: "decision", schema: `{"type":"object","properties":{"op":{"type":"string","enum":["list","propose"]}},"required":["op"]}`,
				}})
				if tc.name == "ask" || tc.name == "proxy_ask" {
					target = NewAskTool()
				}
				reg.Add(target)
				raw := tc.args
				switch format {
				case "object":
					raw = `{"arguments":` + raw + `}`
				case "string":
					quoted, err := json.Marshal(raw)
					if err != nil {
						t.Fatal(err)
					}
					raw = `{"arguments":` + string(quoted) + `}`
				}
				call := provider.ToolCall{ID: "decision", Name: target.Name(), Arguments: raw}
				if tc.name == "proxy_ask" {
					reg.Add(usecap.NewUseCapabilityTool(context.Background(), nil, nil, reg, nil, nil, nil))
					call.Name = "use_capability"
					call.Arguments = `{"action":"call","capability_id":"tool:ask","arguments":` + raw + `}`
				}
				a := New(nil, reg, sessionstore.NewSession(""), Options{}, event.Discard)
				calls := []provider.ToolCall{call, {ID: "later", Name: "read_file", Arguments: `{"path":"later.txt"}`}}
				prepared := a.prepareCallBatch(calls)
				if format != "plain" && prepared[0].ArgumentNormalization == nil {
					t.Fatal("wrapped decision was not normalized")
				}
				wantScheduled := 2
				if tc.barrier {
					wantScheduled = 1
				}
				scheduled, _ := scheduleUpToDecision(reg, prepared)
				if len(scheduled) != wantScheduled {
					t.Errorf("scheduled %d calls, want %d", len(scheduled), wantScheduled)
				}
				out := a.executeOne(context.Background(), &a.turn, call)
				if out.errMsg != "" || out.endsRound != tc.barrier {
					t.Errorf("single call: err=%q endsRound=%v, want %v", out.errMsg, out.endsRound, tc.barrier)
				}
				batch := a.executeBatch(context.Background(), &a.turn, calls)
				if batch.outcomes[0].errMsg != "" || batch.outcomes[0].endsRound != tc.barrier {
					t.Errorf("batch decision outcome: %+v", batch.outcomes[0])
				}
				if len(later.calls) != wantScheduled-1 || batch.outcomes[1].blocked != tc.barrier {
					t.Fatalf("later calls=%v outcome=%+v", later.calls, batch.outcomes[1])
				}
			})
		}
	}
}
