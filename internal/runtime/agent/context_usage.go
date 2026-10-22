package agent

import "reasonix/internal/contract/tool"

// contextUsage memoises the projected prompt size. The estimate walks every
// visible message, and status gauges redraw far more often than the view moves,
// so it is keyed on everything that can change the answer: the transcript,
// projection, calibration, and provider-visible tool schemas.
type contextUsage struct {
	transcriptVersion  uint64
	projectionVersion  uint64
	calibration        *promptTokenCalibration
	anchor             *usageAnchor
	tools              *tool.Registry
	toolSchemaRevision uint64
	tokens             int
}

// ContextUsedTokens is the prompt size the next request sends, measured the way
// the fold trigger measures it: the provider's last reported size plus what the
// view gained since (providerReportedFloor). The hard ceiling reads the estimate
// alone. After a fold or on a rebound session there is no anchor, and the
// estimate stands.
func (a *Agent) ContextUsedTokens() int {
	if a == nil {
		return 0
	}
	return a.window().contextUsedTokens()
}

func (a *contextWindow) contextUsedTokens() int {
	session := a.Session()
	if session == nil {
		return 0
	}
	transcriptVersion := session.TranscriptVersion()
	projectionVersion := a.currentProjectionVersion()
	calibration := a.sess.output.promptCalibration.Load()
	anchor := a.sess.output.usageAnchor.Load()
	tools := a.svc.tools
	toolSchemaRevision := tools.SchemaRevision()
	if cached := a.sess.output.contextUsage.Load(); cached != nil &&
		cached.transcriptVersion == transcriptVersion &&
		cached.projectionVersion == projectionVersion &&
		cached.calibration == calibration &&
		cached.anchor == anchor &&
		cached.tools == tools &&
		cached.toolSchemaRevision == toolSchemaRevision {
		return cached.tokens
	}
	visible := a.modelVisibleMessages()
	tokens := a.promptPressure(a.visibleRequestShape(visible), projectionVersion, a.estimatedVisibleRequestTokens(visible))

	a.sess.output.contextUsage.Store(&contextUsage{
		transcriptVersion:  transcriptVersion,
		projectionVersion:  projectionVersion,
		calibration:        calibration,
		anchor:             anchor,
		tools:              tools,
		toolSchemaRevision: toolSchemaRevision,
		tokens:             tokens,
	})
	return tokens
}

// promptPressure is the one reading of how full the context is, for the gauge and
// for the fold trigger alike: the provider's last reported size plus the growth
// since, where there is an anchor, and the local estimate where there is none (after
// a fold, on a rebound session, before the first usage). The provider's own count
// outranks the estimate, high or low; the hard ceiling reads the estimate alone.
func (a *contextWindow) promptPressure(shape requestCalibrationShape, projectionVersion uint64, estimate int) int {
	if floor, ok := a.providerReportedFloor(shape, projectionVersion); ok {
		return floor
	}
	return estimate
}
