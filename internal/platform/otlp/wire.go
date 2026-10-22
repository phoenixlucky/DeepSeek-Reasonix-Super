package otlp

import (
	"encoding/json"
	"strconv"
)

type jsonValue struct {
	String *string `json:"stringValue,omitempty"`
	Int    *string `json:"intValue,omitempty"`
	Bool   *bool   `json:"boolValue,omitempty"`
}

type jsonAttr struct {
	Key   string    `json:"key"`
	Value jsonValue `json:"value"`
}

type jsonStatus struct {
	Code int `json:"code"`
}

type jsonSpan struct {
	TraceID  string     `json:"traceId"`
	SpanID   string     `json:"spanId"`
	ParentID string     `json:"parentSpanId,omitempty"`
	Name     string     `json:"name"`
	Kind     int        `json:"kind"`
	Start    string     `json:"startTimeUnixNano"`
	End      string     `json:"endTimeUnixNano"`
	Attrs    []jsonAttr `json:"attributes,omitempty"`
	Status   jsonStatus `json:"status"`
}

type jsonScope struct {
	Name string `json:"name"`
}

type jsonScopeSpans struct {
	Scope jsonScope  `json:"scope"`
	Spans []jsonSpan `json:"spans"`
}

type jsonResource struct {
	Attrs []jsonAttr `json:"attributes"`
}

type jsonResourceSpans struct {
	Resource   jsonResource     `json:"resource"`
	ScopeSpans []jsonScopeSpans `json:"scopeSpans"`
}

type jsonBody struct {
	ResourceSpans []jsonResourceSpans `json:"resourceSpans"`
}

func toJSONAttr(a Attr) jsonAttr {
	out := jsonAttr{Key: a.Key}
	switch a.kind {
	case 'i':
		s := strconv.FormatInt(a.num, 10)
		out.Value.Int = &s
	case 'b':
		b := a.flag
		out.Value.Bool = &b
	default:
		s := a.str
		out.Value.String = &s
	}
	return out
}

func toJSONSpan(s Span) jsonSpan {
	out := jsonSpan{TraceID: s.TraceID, SpanID: s.SpanID, ParentID: s.ParentID, Name: s.Name, Kind: s.Kind,
		Start: strconv.FormatInt(s.StartNano, 10), End: strconv.FormatInt(s.EndNano, 10), Status: jsonStatus{Code: 1}}
	if s.Failed {
		out.Status.Code = 2
	}
	for _, a := range s.Attrs {
		out.Attrs = append(out.Attrs, toJSONAttr(a))
	}
	return out
}

// encodeBatch is the OTLP/HTTP JSON body for spans under one resource.
func encodeBatch(resource []Attr, spans []Span) ([]byte, error) {
	res := jsonResource{Attrs: []jsonAttr{}}
	for _, a := range resource {
		res.Attrs = append(res.Attrs, toJSONAttr(a))
	}
	body := jsonBody{ResourceSpans: []jsonResourceSpans{{Resource: res, ScopeSpans: []jsonScopeSpans{{Scope: jsonScope{Name: "reasonix"}}}}}}
	for _, s := range spans {
		body.ResourceSpans[0].ScopeSpans[0].Spans = append(body.ResourceSpans[0].ScopeSpans[0].Spans, toJSONSpan(s))
	}
	return json.Marshal(body)
}
