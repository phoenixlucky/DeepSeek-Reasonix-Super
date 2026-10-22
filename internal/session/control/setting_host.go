package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/settings"
	"reasonix/internal/contract/tool"
)

const (
	// SettingApprovalTool is the card for a Preference setting; a session grant
	// covers that one setting.
	SettingApprovalTool = "setting_change"
	// SettingElevatedApprovalTool is the card for an Elevated setting. It offers
	// no session grant, so every change of it is a fresh click.
	SettingElevatedApprovalTool = "setting_change_elevated"

	settingProposalsPerTurn = 3
	settingReasonRunes      = 160
	settingListRows         = 60
	// settingValueRunes bounds the new value so the card can show all of it: what
	// the user reads is exactly what is written. settingRawBytes bounds what is
	// parsed at all.
	settingValueRunes = 400
	settingRawBytes   = 4 << 10
)

// Setting refusal codes the controller adds to the declaration's own.
const (
	CodeSettingDeclined   = "setting.declined"
	CodeSettingNoAnswer   = "setting.no_approver"
	CodeSettingLimit      = "setting.proposal_limit"
	CodeSettingUnwritable = "setting.unwritable"
	CodeSettingTooLong    = "setting.value_too_long"
	CodeSettingPlanMode   = "setting.plan_mode"
)

// settingsState holds the per-turn count of proposals, so a model cannot turn
// the card into a stream the user answers to get on with their work, and the
// settings the user agreed to for the rest of the session. The grants are kept
// here and not in the approval manager's, whose rule for a tool it does not
// know covers the whole tool: one agreed setting would cover them all.
type settingsState struct {
	proposals atomic.Int32
	mu        sync.Mutex
	granted   map[string]bool
}

func (s *settingsState) grant(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.granted == nil {
		s.granted = map[string]bool{}
	}
	s.granted[id] = true
}

func (s *settingsState) granting(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.granted[id]
}

func (c *Controller) startSettings(link *tool.SettingsLink) {
	if link != nil {
		link.Bind(c)
	}
}

// SettingsList finds declared settings whose id holds every word of query, with
// the value the user file holds now for those whose value may be shown.
func (c *Controller) SettingsList(query string) []tool.SettingInfo {
	words := strings.Fields(strings.ToLower(query))
	cfg := config.LoadForEdit(config.UserConfigPath())
	var out []tool.SettingInfo
	for _, e := range settings.All() {
		id := strings.ToLower(e.ID)
		match := true
		for _, w := range words {
			match = match && strings.Contains(id, w)
		}
		if !match {
			continue
		}
		row := tool.SettingInfo{ID: e.ID, Class: e.Class.String(), Effect: e.Effect.String()}
		if e.Class != settings.Guarded {
			if v, err := settings.Read(cfg, e.ID); err == nil {
				row.Value = v
			}
		}
		out = append(out, row)
		if len(out) > settingListRows {
			break
		}
	}
	return out
}

// OpenSetting asks the frontend to bring the user to a setting. It changes
// nothing, which is why it is the one operation a Guarded setting allows.
func (c *Controller) OpenSetting(id string) error {
	if _, ok := settings.Lookup(id); !ok {
		return settingRefusal(&settings.Error{Code: settings.CodeUnknown, ID: id})
	}
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeSettingOpen,
		Text: fmt.Sprintf(i18n.M.SettingOpenFmt, id), Detail: id})
	return nil
}

// ProposeSetting asks the user to confirm a change and applies it only after
// their answer. The card is worded from the declaration; the model's reason is
// quoted beneath it.
func (c *Controller) ProposeSetting(ctx context.Context, id string, raw json.RawMessage, reason string) (tool.SettingReceipt, error) {
	e, ok := settings.Lookup(id)
	if !ok {
		return tool.SettingReceipt{}, settingRefusal(&settings.Error{Code: settings.CodeUnknown, ID: id})
	}
	if e.Class == settings.Guarded {
		return tool.SettingReceipt{}, tool.Refusal{Code: settings.CodeGuarded, Remedy: settings.RemedyFor(id, nil),
			Message: id + " is a boundary setting; only the user changes it."}
	}
	if c.PlanMode() {
		return tool.SettingReceipt{}, tool.Refusal{Code: CodeSettingPlanMode, Message: "Settings are not changed while plan mode is on: use op=open to point the user at it, or ask again once the plan is approved."}
	}
	if len(raw) > settingRawBytes {
		return tool.SettingReceipt{}, tooLong(id)
	}
	probe := config.LoadForEdit(config.UserConfigPath())
	from, to, err := settings.Write(probe, id, raw)
	if err != nil {
		return tool.SettingReceipt{}, settingRefusal(err)
	}
	if utf8.RuneCountInString(whole(to)) > settingValueRunes {
		return tool.SettingReceipt{}, tooLong(id)
	}
	receipt := tool.SettingReceipt{ID: id, From: from, To: to, Effect: e.Effect.String()}
	if reflect.DeepEqual(from, to) {
		receipt.Via = "unchanged"
		return receipt, nil
	}
	if c.settings.proposals.Add(1) > settingProposalsPerTurn {
		return tool.SettingReceipt{}, tool.Refusal{Code: CodeSettingLimit,
			Message: "Enough settings have been proposed this turn; carry on and ask again next turn if it still matters."}
	}
	via, err := c.confirmSetting(ctx, e, receipt, reason)
	if err != nil {
		return tool.SettingReceipt{}, err
	}
	receipt.Via = via
	from, to, err = c.writeSetting(id, raw)
	if err != nil {
		return tool.SettingReceipt{}, err
	}
	receipt.From, receipt.To = from, to
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeSettingChanged,
		Text: fmt.Sprintf(i18n.M.SettingChangedFmt, id, shown(from), whole(to), effectSentence(e.Effect)), Detail: id})
	return receipt, nil
}

// confirmSetting returns how the change was agreed. A grant the user gave
// earlier for this one Preference setting counts; nothing else answers.
func (c *Controller) confirmSetting(ctx context.Context, e settings.Entry, r tool.SettingReceipt, reason string) (string, error) {
	card := SettingApprovalTool
	if e.Class == settings.Elevated {
		card = SettingElevatedApprovalTool
	}
	args, _ := json.Marshal(map[string]any{"id": e.ID, "from": r.From, "to": r.To, "effect": r.Effect})
	text := fmt.Sprintf(i18n.M.SettingApprovalFmt, e.ID, shown(r.From), whole(r.To), effectSentence(e.Effect))
	if reason = tidyReason(reason); reason != "" {
		text += "\n" + fmt.Sprintf(i18n.M.SettingReasonFmt, reason)
	}
	if e.Class == settings.Preference && c.settings.granting(e.ID) {
		return "session_grant", nil
	}
	reply, err := c.requestApprovalDecision(ctx, approvalRequest{tool: card, subject: e.ID, args: args, reason: text, fresh: true})
	switch {
	case err != nil && ctx.Err() != nil:
		return "", ctx.Err()
	case err != nil:
		return "", tool.Refusal{Code: CodeSettingNoAnswer, Message: "Nobody answered the request to change " + e.ID + "."}
	case !reply.allow:
		return "", tool.Refusal{Code: CodeSettingDeclined, Message: "The user declined changing " + e.ID + "; leave it as it is."}
	}
	if reply.session && e.Class == settings.Preference {
		c.settings.grant(e.ID)
	}
	return "click", nil
}

// writeSetting applies the change to the user file. It re-reads the file under
// the edit lock and refuses to write over one it cannot read.
func (c *Controller) writeSetting(id string, raw json.RawMessage) (from, to any, err error) {
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg, err := config.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return nil, nil, tool.Refusal{Code: CodeSettingUnwritable, Message: "The user config could not be read, so nothing was changed."}
	}
	if from, to, err = settings.Write(cfg, id, raw); err != nil {
		return nil, nil, settingRefusal(err)
	}
	if err := cfg.SaveTo(path); err != nil {
		return nil, nil, tool.Refusal{Code: CodeSettingUnwritable, Message: "The user config could not be saved, so nothing was changed."}
	}
	return from, to, nil
}

func settingRefusal(err error) error {
	var se *settings.Error
	if !errors.As(err, &se) {
		return err
	}
	r := tool.Refusal{Code: se.Code, Message: se.ID + ": " + se.Message()}
	if se.Code == settings.CodeShape || se.Code == settings.CodeSecret {
		r.Remedy = &tool.Remedy{Op: tool.RemedyOpen, Setting: se.ID}
	}
	return r
}

func effectSentence(e settings.Effect) string {
	switch e {
	case settings.Immediate:
		return i18n.M.SettingEffectImmediate
	case settings.NextTurn:
		return i18n.M.SettingEffectNextTurn
	case settings.Restart:
		return i18n.M.SettingEffectRestart
	}
	return i18n.M.SettingEffectOnOpen
}

func tooLong(id string) error {
	return tool.Refusal{Code: CodeSettingTooLong, Message: fmt.Sprintf("The value for %s is longer than the confirmation card can show whole (%d characters); propose a shorter one.", id, settingValueRunes)}
}

// whole is a value as the card prints it. A value that is written is never cut.
func whole(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	return string(b)
}

// shown is a value that is only being replaced, so it may be shortened.
func shown(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	s := string(b)
	if utf8.RuneCountInString(s) > 80 {
		s = string([]rune(s)[:80]) + "…"
	}
	return s
}

func tidyReason(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > settingReasonRunes {
		s = string([]rune(s)[:settingReasonRunes]) + "…"
	}
	return s
}
