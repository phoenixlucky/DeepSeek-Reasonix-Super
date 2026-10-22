package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"

	"reasonix/internal/base/textutil"
	"reasonix/internal/contract/event"
)

// recordFollowed writes the definitions a followed server just changed as the
// approved ones. When the record cannot be written the changes stay held, so a
// followed change is never accepted without leaving the record that says so.
func recordFollowed(path string, pins toolPinFile, followed []HeldTool) bool {
	next := make(map[string]string, len(pins.Tools)+len(followed))
	maps.Copy(next, pins.Tools)
	for _, h := range followed {
		next[h.RawName] = h.Digest
	}
	return writeToolPins(path, next, true) == nil
}

func readToolPins(s Spec) (toolPinFile, string, error) {
	path := toolPinsPath(s)
	if path == "" {
		return toolPinFile{}, "", errors.New("no state directory to record the choice in")
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return toolPinFile{Version: toolPinsVersion, Tools: map[string]string{}}, path, nil
	}
	if err != nil {
		return toolPinFile{}, path, err
	}
	var pins toolPinFile
	if json.Unmarshal(raw, &pins) != nil || pins.Version != toolPinsVersion || pins.Tools == nil {
		return toolPinFile{}, path, errors.New("the approval record cannot be read")
	}
	return pins, path, nil
}

// ToolPinsFollow reports whether the user chose to take this server's tool
// definition changes without being asked.
func ToolPinsFollow(s Spec) bool {
	pins, _, err := readToolPins(s)
	return err == nil && pins.Follow
}

// SetToolPinsFollow records that choice for one server. It changes what is asked,
// not what is shown: every change taken this way is announced, and turning it
// off puts the next change back behind the user's answer. A record that cannot
// be read is never overwritten by it.
func SetToolPinsFollow(s Spec, follow bool) error {
	pins, path, err := readToolPins(s)
	if err != nil {
		return err
	}
	if pins.Follow == follow {
		return nil
	}
	return writeToolPins(path, pins.Tools, follow)
}

// announceFollowed tells the operator which definitions a followed server
// changed, once.
func (h *Host) announceFollowed(c *Client) {
	c.toolsMu.Lock()
	followed := c.followed
	c.followed = nil
	c.toolsMu.Unlock()
	if len(followed) == 0 {
		return
	}
	h.statusMu.RLock()
	sink := h.statusSink
	h.statusMu.RUnlock()
	if sink == nil {
		return
	}
	names := make([]string, len(followed))
	for i, t := range followed {
		names[i] = t.RawName
	}
	server := textutil.ShownLocator(c.spec.Name)
	sink.Emit(event.Event{
		Kind: event.Notice, Audience: event.NoticeAudienceOperator, Level: event.LevelInfo,
		Code:   event.NoticeCodeMCPToolsFollowed,
		Text:   fmt.Sprintf("MCP server %q: %d tool definition(s) changed or were added and were accepted because you chose to follow this server's changes.", server, len(followed)),
		Detail: textutil.ShownLocator(strings.Join(names, ", ")),
	})
}
