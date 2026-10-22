package responses

func applySummaryToolChoice(body map[string]any, summary bool) {
	if summary {
		body["tool_choice"] = "none"
	}
}
