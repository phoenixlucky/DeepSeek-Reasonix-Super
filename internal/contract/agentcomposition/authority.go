package agentcomposition

import (
	"fmt"
	"strings"
)

// History decoding checks structure; executable declarations also obey source authority.
func (d Definition) ValidateForExecution() error {
	if err := d.Validate(); err != nil {
		return err
	}
	if !strings.HasPrefix(d.ID, "project:") {
		return nil
	}
	for _, selection := range []Selection{d.MCP, d.Extensions} {
		for _, module := range selection.Members {
			p, err := ParseParameters(module.Parameters)
			if err != nil {
				return err
			}
			if p.Args != nil || p.URL != nil || p.Env != nil || p.Headers != nil {
				return fmt.Errorf("%w: project module %s may override only timeouts", ErrInvalid, module.ID)
			}
		}
	}
	return nil
}

func (b *Binding) ValidateForExecution() error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := b.Version.Definition.ValidateForExecution(); err != nil {
		return err
	}
	return b.Resolved.ValidateForExecution()
}
