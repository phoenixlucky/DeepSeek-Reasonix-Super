// Package delivery defines persisted file declarations and their UI projection.
package delivery

import (
	"encoding/json"
	"path/filepath"
)

const (
	ToolName = "present_files"
	Identity = "tool:present_files"
	MaxFiles = 4
)

type File struct {
	Path        string `json:"path"`
	Root        string `json:"root,omitempty"`
	Description string `json:"description,omitempty"`
}

type Result struct {
	Version int    `json:"version"`
	Files   []File `json:"files"`
}

// Decode requires the host's resolved builtin identity and a successful result.
func Decode(name, identity string, failed bool, output string) []File {
	if failed || name != ToolName || identity != Identity || len(output) > 32<<10 {
		return nil
	}
	var result Result
	if json.Unmarshal([]byte(output), &result) != nil || result.Version != 1 || len(result.Files) == 0 || len(result.Files) > MaxFiles {
		return nil
	}
	for _, file := range result.Files {
		if !filepath.IsLocal(filepath.FromSlash(file.Path)) || file.Path == "." || len(file.Description) > 1024 {
			return nil
		}
	}
	return result.Files
}
