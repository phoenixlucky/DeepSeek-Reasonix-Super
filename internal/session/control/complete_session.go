package control

import (
	"sort"
	"strings"
	"time"

	"reasonix/internal/state/sessionstore"
)

// maxSessionRefItems keeps the menu to what fits a glance; the rail's search is
// where the whole list is.
const maxSessionRefItems = 20

// SessionOption is a past conversation of this workspace the composer may offer
// to reference. Title is what was said about it, At when it was last active.
type SessionOption struct {
	ID    string
	Title string
	At    time.Time
	Turns int
}

// ActiveSessionToken finds the #-word under the byte offset cursor. The '#' must
// start the line or follow whitespace, so a heading mark or an issue number in
// the middle of a word never opens the menu. [from, to) is the whole word;
// query stops at the caret.
func ActiveSessionToken(line string, cursor int) (from, to int, query string, ok bool) {
	if cursor < 0 || cursor > len(line) {
		cursor = len(line)
	}
	for i := cursor - 1; i >= 0; i-- {
		switch line[i] {
		case ' ', '\t', '\n':
			return 0, 0, "", false
		case '#':
			if i > 0 && line[i-1] != ' ' && line[i-1] != '\t' && line[i-1] != '\n' {
				return 0, 0, "", false
			}
			to = i + 1
			for to < len(line) && line[to] != ' ' && line[to] != '\t' && line[to] != '\n' {
				to++
			}
			return i, to, line[i+1 : min(max(cursor, i+1), to)], true
		}
	}
	return 0, 0, "", false
}

// SessionRefItems are the options whose title holds the query, newest first.
// The id is not searched: an issue number typed after a '#' would otherwise
// find a conversation by its timestamp and take the next Enter.
func SessionRefItems(options []SessionOption, query string) []SlashItem {
	needle := strings.ToLower(strings.TrimSpace(query))
	var items []SlashItem
	for _, o := range options {
		title := strings.TrimSpace(o.Title)
		if title == "" {
			title = o.ID
		}
		if needle != "" && !strings.Contains(strings.ToLower(title), needle) {
			continue
		}
		items = append(items, SlashItem{Label: title, Insert: "@" + sessionRefPrefix + o.ID, Hint: o.At.Local().Format("2006-01-02 15:04"), Kind: "session"})
		if len(items) == maxSessionRefItems {
			break
		}
	}
	return items
}

// sessionOptions lists this workspace's saved conversations, newest first,
// leaving out the one being written.
func (c *Controller) sessionOptions() []SessionOption {
	dir := c.SessionDir()
	if dir == "" {
		return nil
	}
	infos, err := sessionstore.ListSessions(dir)
	if err != nil {
		return nil
	}
	current := ""
	if p := c.SessionPath(); p != "" {
		current = sessionstore.CanonicalSessionPath(p)
	}
	var out []SessionOption
	for _, info := range infos {
		if current != "" && sessionstore.CanonicalSessionPath(info.Path) == current {
			continue
		}
		out = append(out, SessionOption{
			ID:    sessionstore.BranchID(info.Path),
			Title: firstNonEmpty(info.CustomTitle, info.TopicTitle, info.Preview),
			At:    info.LastActivityAt,
			Turns: info.Turns,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}
