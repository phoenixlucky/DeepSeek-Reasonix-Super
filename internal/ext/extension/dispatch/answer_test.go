package dispatch

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/ext/extension"
	"reasonix/internal/ext/extension/protocol"
)

func answerDispatcher(point extension.InterceptorPoint, answer protocol.InterceptResult, warns *warnRecorder) *Dispatcher {
	fake := &fakeClient{interceptFn: func(protocol.InterceptEvent, json.RawMessage) (protocol.InterceptResult, error) { return answer, nil }}
	return New(map[extension.InterceptorPoint][]extension.Contribution{point: {interceptor("mod-x", point, 0)}}, nil,
		func(string) Client { return fake }, nil, Options{Warn: warns.warn})
}

// An answer stands in for the tool's output, so it means something only before
// the tool would have run; anywhere else it is a contract violation, and the
// ruling is dropped.
func TestAnAnswerIsLegalOnlyAtToolBefore(t *testing.T) {
	ans := protocol.InterceptResult{Decision: protocol.DecisionBlock, Answer: "42"}

	d := answerDispatcher(extension.PointToolBefore, ans, &warnRecorder{})
	got, err := d.Intercept(context.Background(), extension.PointToolBefore, &ToolBeforePayload{Name: "bash", Arguments: `{}`})
	if err != nil || got.Answer != "42" || got.AnswerBy != "mod-x" || !got.Blocked {
		t.Fatalf("tool.before answer = %+v, %v", got, err)
	}

	warns := &warnRecorder{}
	d = answerDispatcher(extension.PointToolAfter, ans, warns)
	got, err = d.Intercept(context.Background(), extension.PointToolAfter, &ToolAfterPayload{Name: "bash"})
	if err != nil || got.Answer != "" || got.Blocked || warns.count() != 1 {
		t.Fatalf("tool.after answer = %+v, %v, warnings %d; want it dropped with one warning", got, err, warns.count())
	}
}

// An answer rides only a block: any other decision carrying one is malformed.
func TestAnAnswerWithoutABlockIsAViolation(t *testing.T) {
	warns := &warnRecorder{}
	d := answerDispatcher(extension.PointToolBefore, protocol.InterceptResult{Decision: protocol.DecisionContinue, Answer: "42"}, warns)
	got, err := d.Intercept(context.Background(), extension.PointToolBefore, &ToolBeforePayload{Name: "bash", Arguments: `{}`})
	if err != nil || got.Answer != "" || warns.count() != 1 {
		t.Fatalf("continue with an answer = %+v, %v, warnings %d", got, err, warns.count())
	}
}
