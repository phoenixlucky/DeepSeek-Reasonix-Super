package jobs

import (
	"io"
	"os"
	"unicode/utf8"
)

// Peek is the end of one job's output as a person looks at it. Looking is not
// reading: it moves nothing the model's own reader has not yet seen.
type Peek struct {
	Text      string
	Status    Status
	Bytes     int64 // everything the job has written so far
	Truncated bool  // Text is only the last part of it
}

// PeekForSession returns up to max bytes from the end of a job's output, when
// id belongs to parentSession. Unlike OutputForSession it never advances the
// offset bash_output reads from, so the panel can be refreshed as often as it
// likes without taking output away from the model.
func (m *Manager) PeekForSession(parentSession, id string, max int) (Peek, bool) {
	j := m.get(parentSession, id)
	if j == nil {
		return Peek{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := Peek{Status: j.status}
	var raw []byte
	if j.artifact.path != "" {
		raw, out.Bytes = fileEnd(j.artifact.path, max)
	} else {
		out.Bytes = int64(len(j.tail))
		raw = j.tail[max0(len(j.tail)-max):]
	}
	out.Truncated = int64(len(raw)) < out.Bytes
	out.Text = string(raw[firstRuneStart(raw):])
	return out, true
}

func max0(n int) int { return max(n, 0) }

// fileEnd reads the end of a file without reading the rest of it.
func fileEnd(path string, max int) ([]byte, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0
	}
	size := info.Size()
	from := max0(int(size) - max)
	if _, err := f.Seek(int64(from), io.SeekStart); err != nil {
		return nil, size
	}
	b, _ := io.ReadAll(io.LimitReader(f, int64(max)))
	return b, size
}

// firstRuneStart skips the bytes of a character the cut went through.
func firstRuneStart(b []byte) int {
	for i := 0; i < len(b) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(b[i]) {
			return i
		}
	}
	return 0
}
