package jobs

import "strings"

// View is a read-only snapshot of a job for the status bar.
type View struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// Description is the short name the caller gave it for a person, Detail the
	// full command or task it runs; Label stays the short preview.
	Description string `json:"description,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Status      string `json:"status"`
	StartedAt   int64  `json:"startedAt"` // unix milliseconds
}

// Running returns a snapshot of the still-running jobs (for the status bar).
func (m *Manager) Running() []View {
	return m.RunningForSession("")
}

// RunningForSession returns still-running jobs owned by parentSession. Empty
// parentSession preserves the legacy unscoped behavior.
func (m *Manager) RunningForSession(parentSession string) []View {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []View
	for _, key := range m.order {
		j := m.jobs[key]
		if !sessionMatches(parentSession, j.SessionID) {
			continue
		}
		select {
		case <-j.done:
			continue
		default:
		}
		j.mu.Lock()
		// A requested stop keeps the job on the running surface until its done
		// channel closes, so a rebuild or workspace lease cannot see an idle runtime
		// while the process tree is still unwinding.
		out = append(out, View{ID: j.ID, Kind: j.Kind, Label: j.Label, Description: j.Present.Description, Detail: j.Present.Detail, Status: string(Running), StartedAt: j.times.started})
		j.mu.Unlock()
	}
	return out
}

// Presentation is what a job is called, and what it runs in full, for a person.
type Presentation struct {
	Description string
	Detail      string
}

// StartOption sets what a job carries beyond its kind and label, before it is
// visible to anything else.
type StartOption func(*Job)

// MaxDetailRunes bounds the full command a job keeps for display, and
// MaxDescriptionRunes the one-line name the model gave it.
const (
	MaxDetailRunes      = 4096
	MaxDescriptionRunes = 200
)

func bounded(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[:limit]) + "…"
}

// WithPresentation names the job for a person and records what it runs in full.
func WithPresentation(description, detail string) StartOption {
	return func(j *Job) {
		j.Present.Description = bounded(description, MaxDescriptionRunes)
		j.Present.Detail = bounded(detail, MaxDetailRunes)
	}
}

func applyOptions(j *Job, opts []StartOption) {
	for _, opt := range opts {
		opt(j)
	}
}
