package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"reasonix/internal/contract/tool"
)

// What the host keeps of the external text a task has read, and how it matches a
// later call against it. Matching is exact bytes: no normalising, decoding or
// stemming, so it never reads wording and never claims more than a copy.
const (
	// originResultMax bounds what is kept of one result, originTaskMax of a task.
	originResultMax = 64 << 10
	originTaskMax   = 1 << 20
	// originMinLeaf is the shortest argument string that says anything.
	originMinLeaf = 12
)

type originRecord struct {
	origin tool.Provenance
	text   string
}

// originLedger holds the text of this task's external results host-side, apart
// from the messages, so a fold or a replay cannot erase it. It lives and dies with
// the task; a resumed session starts empty, which is a false negative and is
// documented as one rather than rebuilt by reading message text.
type originLedger struct {
	mu       sync.Mutex
	recs     []originRecord
	bytes    int
	recorded int
	matched  int
}

// note keeps the text of an external result, bounded, dropping the oldest to
// stay under the task ceiling. A workspace result is never kept.
func (l *originLedger) note(p tool.Provenance, text string) {
	if l == nil || !p.External() || text == "" {
		return
	}
	if len(text) > originResultMax {
		text = text[:originResultMax]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recs = append(l.recs, originRecord{origin: p, text: text})
	l.bytes += len(text)
	l.recorded++
	for l.bytes > originTaskMax && len(l.recs) > 1 {
		l.bytes -= len(l.recs[0].text)
		l.recs = l.recs[1:]
	}
}

// match returns the origins whose recorded text contains a string leaf of args
// verbatim, leaving out any leaf the user also wrote.
func (l *originLedger) match(args json.RawMessage, userTexts func() []string) []tool.Provenance {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	recs := append([]originRecord(nil), l.recs...)
	l.mu.Unlock()
	if len(recs) == 0 {
		return nil
	}
	var doc any
	if json.Unmarshal(args, &doc) != nil {
		return nil
	}
	var out []tool.Provenance
	var users []string
	for _, leaf := range stringLeaves(doc, nil) {
		if len(leaf) < originMinLeaf {
			continue
		}
		var from []tool.Provenance
		for _, r := range recs {
			if strings.Contains(r.text, leaf) {
				from = appendOrigin(from, r.origin)
			}
		}
		if len(from) == 0 {
			continue
		}
		if users == nil && userTexts != nil {
			users = userTexts()
		}
		if userWrote(users, leaf) {
			continue
		}
		for _, p := range from {
			out = appendOrigin(out, p)
		}
	}
	return out
}

func stringLeaves(v any, into []string) []string {
	switch x := v.(type) {
	case string:
		return append(into, x)
	case []any:
		for _, e := range x {
			into = stringLeaves(e, into)
		}
	case map[string]any:
		for _, e := range x {
			into = stringLeaves(e, into)
		}
	}
	return into
}

func appendOrigin(list []tool.Provenance, p tool.Provenance) []tool.Provenance {
	if slices.Contains(list, p) {
		return list
	}
	return append(list, p)
}

func userWrote(users []string, leaf string) bool {
	for _, u := range users {
		if strings.Contains(u, leaf) {
			return true
		}
	}
	return false
}

// noteMatch counts a call whose arguments carried recorded external text.
func (l *originLedger) noteMatch() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.matched++
	l.mu.Unlock()
}

// take returns the counts since the last take and starts them over: content-free
// numbers for the turn's audit.
func (l *originLedger) take() (recorded, matched int) {
	if l == nil {
		return 0, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	recorded, matched = l.recorded, l.matched
	l.recorded, l.matched = 0, 0
	return recorded, matched
}
