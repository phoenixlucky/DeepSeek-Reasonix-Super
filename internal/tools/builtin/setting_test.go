package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/contract/tool"
)

type fakeSettings struct {
	proposed []string
	opened   []string
	rows     []tool.SettingInfo
}

func (f *fakeSettings) SettingsList(string) []tool.SettingInfo { return f.rows }
func (f *fakeSettings) ProposeSetting(_ context.Context, id string, v json.RawMessage, _ string) (tool.SettingReceipt, error) {
	f.proposed = append(f.proposed, id+"="+string(v))
	return tool.SettingReceipt{ID: id, From: false, To: true, Effect: "next_turn", Via: "click"}, nil
}
func (f *fakeSettings) OpenSetting(id string) error { f.opened = append(f.opened, id); return nil }

func settingToolWith(h tool.SettingsHost) tool.Tool {
	link := &tool.SettingsLink{}
	if h != nil {
		link.Bind(h)
	}
	return SettingTool(link)
}

func TestSettingProposeReachesTheHostAndReturnsTheReceipt(t *testing.T) {
	h := &fakeSettings{}
	out, err := settingToolWith(h).Execute(context.Background(), json.RawMessage(`{"op":"propose","id":"peers.dispatch","value":true,"reason":"hand a task over"}`))
	if err != nil || len(h.proposed) != 1 || h.proposed[0] != "peers.dispatch=true" {
		t.Fatalf("proposed=%v err=%v", h.proposed, err)
	}
	if !strings.Contains(out, `"via":"click"`) || !strings.Contains(out, `"effect":"next_turn"`) {
		t.Fatalf("receipt = %s", out)
	}
}

func TestSettingNeedsAValueToPropose(t *testing.T) {
	h := &fakeSettings{}
	if _, err := settingToolWith(h).Execute(context.Background(), json.RawMessage(`{"op":"propose","id":"peers.dispatch"}`)); err == nil || len(h.proposed) != 0 {
		t.Fatalf("proposed %v with no value, err=%v", h.proposed, err)
	}
}

func TestSettingListIsBoundedAndSaysSo(t *testing.T) {
	h := &fakeSettings{}
	for range settingListMax + 5 {
		h.rows = append(h.rows, tool.SettingInfo{ID: "a.b", Class: "preference", Effect: "immediate"})
	}
	out, err := settingToolWith(h).Execute(context.Background(), json.RawMessage(`{"op":"list","query":"a"}`))
	var got struct {
		Settings  []tool.SettingInfo
		Truncated bool
	}
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || len(got.Settings) != settingListMax || !got.Truncated {
		t.Fatalf("n=%d truncated=%v err=%v", len(got.Settings), got.Truncated, err)
	}
}

func TestSettingOpenNeverProposes(t *testing.T) {
	h := &fakeSettings{}
	if _, err := settingToolWith(h).Execute(context.Background(), json.RawMessage(`{"op":"open","id":"permissions.mode","value":"yolo"}`)); err != nil || len(h.proposed) != 0 || len(h.opened) != 1 {
		t.Fatalf("proposed=%v opened=%v err=%v", h.proposed, h.opened, err)
	}
}

func TestSettingWithoutAHostRefusesByCode(t *testing.T) {
	_, err := settingToolWith(nil).Execute(context.Background(), json.RawMessage(`{"op":"list"}`))
	var r tool.Refusal
	if !errors.As(err, &r) || r.Code != "setting.unavailable" {
		t.Fatalf("err = %v", err)
	}
}

func TestOnlyAProposalIsADecisionBarrier(t *testing.T) {
	st := settingToolWith(nil)
	if !tool.IsDecisionBarrierCall(st, json.RawMessage(`{"op":"propose"}`)) || tool.IsDecisionBarrierCall(st, json.RawMessage(`{"op":"list"}`)) {
		t.Fatal("barrier follows the op wrongly")
	}
}

func TestTheSettingToolStaysOutOfAReadOnlyChild(t *testing.T) {
	m, ok := settingToolWith(nil).(tool.ReadOnlyExecutionHostMutation)
	if !ok || !m.ReadOnlyExecutionHostMutation() {
		t.Fatal("a read-only child could be handed the setting tool")
	}
}
