package anthropic

type toolChoice struct {
	Type string `json:"type"`
}

func (r anthRequest) forSummary(summary bool) anthRequest {
	if summary && len(r.Tools) > 0 {
		r.ToolChoice = &toolChoice{Type: "none"}
	}
	return r
}
