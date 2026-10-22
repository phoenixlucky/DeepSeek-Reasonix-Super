package agentcomposition

import (
	"fmt"
	"slices"
)

const LiveBindingVersion = 2
const LiveResources = "live"

func (b *Binding) Live() bool {
	return b != nil && b.FormatVersion == LiveBindingVersion && b.ResourceMode == LiveResources
}

func (b *Binding) validateResourceFormat() error {
	if b.FormatVersion == 0 && b.ResourceMode == "" {
		return nil
	}
	if !b.Live() || len(b.Resources) != 0 {
		return fmt.Errorf("%w: unsupported binding resource format", ErrInvalid)
	}
	return nil
}

func (s Selection) Allows(id string) bool {
	return s.Mode == Inherit || s.Mode == Select && slices.ContainsFunc(s.Members, func(m Module) bool { return m.ID == id })
}

type Summary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Revision      string `json:"revision"`
	Digest        string `json:"digest"`
	Started       bool   `json:"started"`
	FormatVersion int    `json:"formatVersion,omitempty"`
	ResourceMode  string `json:"resourceMode,omitempty"`
}

func (b *Binding) Summary() *Summary {
	if b == nil {
		return nil
	}
	return &Summary{
		ID: b.Version.Definition.ID, Name: b.Version.Definition.Name,
		Revision: b.Version.Revision, Digest: b.Digest, Started: b.Started,
		FormatVersion: b.FormatVersion, ResourceMode: b.ResourceMode,
	}
}
