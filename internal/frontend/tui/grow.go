package tui

import "strings"

// growing is a string that takes appends without copying what it holds each
// time. A streamed answer, its thinking and a running command's output are all
// appended to in many small pieces, and `s += piece` copies the whole of s, so
// a long stream cost time and memory quadratic in its length.
type growing struct {
	b    strings.Builder
	last string
}

// growKey names one growing field of one row.
type growKey struct {
	id    int
	field byte
}

const (
	fieldText byte = iota
	fieldReasoning
	fieldStreamed
)

// appendTo adds piece to *dst. The buffer is kept beside the row, and it is
// trusted only while *dst is still exactly what it last handed out: a row whose
// text was replaced meanwhile starts a fresh buffer from the replacement.
func (t *Transcript) appendTo(id int, field byte, dst *string, piece string) {
	key := growKey{id, field}
	g := t.grown[key]
	if g == nil || *dst != g.last {
		if t.grown == nil {
			t.grown = map[growKey]*growing{}
		}
		g = &growing{}
		g.b.Grow(len(*dst) + len(piece))
		g.b.WriteString(*dst)
		t.grown[key] = g
	}
	g.b.WriteString(piece)
	g.last = g.b.String()
	*dst = g.last
}
