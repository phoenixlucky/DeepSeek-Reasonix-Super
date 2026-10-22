package agent

import (
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
)

// userAddresses lists the https addresses the user wrote in this conversation:
// the text they typed, never a tool result, the model's own words or what the
// host added around their message.
func (a *Agent) userAddresses() []string {
	var out []string
	for _, m := range a.sess.conversation.Snapshot() {
		if m.Role != provider.RoleUser || m.HostAuthored || m.LocalOnly {
			continue
		}
		text := m.RawContent
		if text == "" {
			text = m.Content
		}
		out = append(out, tool.AddressesIn(text)...)
	}
	return out
}
