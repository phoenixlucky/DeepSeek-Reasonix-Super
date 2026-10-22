package otlp

import (
	"regexp"
	"slices"
)

// Attr is one span attribute. Only the constructors below make one, and the
// string constructors accept a Safe value, so a caller cannot hand free text
// to an attribute without naming where it was made safe.
type Attr struct {
	Key  string
	str  string
	num  int64
	flag bool
	kind byte
}

// Safe is text that is a name or a closed-set value, never content.
type Safe string

var safeName = regexp.MustCompile(`^[A-Za-z0-9_.:/@+-]{1,96}$`)

// Name admits a model, provider or tool name only when it has the shape of one;
// anything else, including text a model wrote into a tool-call name, becomes
// "unknown".
func Name(s string) Safe {
	if safeName.MatchString(s) {
		return Safe(s)
	}
	return "unknown"
}

// OneOf admits s only when it is in the closed set, else "other".
func OneOf(s string, set ...string) Safe {
	if slices.Contains(set, s) {
		return Safe(s)
	}
	return "other"
}

func Str(key string, v Safe) Attr  { return Attr{Key: key, str: string(v), kind: 's'} }
func Int(key string, v int64) Attr { return Attr{Key: key, num: v, kind: 'i'} }
func Bool(key string, v bool) Attr { return Attr{Key: key, flag: v, kind: 'b'} }

// Span is one finished span, in the shape OTLP carries.
type Span struct {
	TraceID, SpanID, ParentID string
	Name                      string
	Kind                      int
	StartNano, EndNano        int64
	Attrs                     []Attr
	Failed                    bool
}

const (
	kindInternal = 1
	kindClient   = 3
)
